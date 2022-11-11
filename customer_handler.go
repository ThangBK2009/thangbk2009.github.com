package handler

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	cutils "common-libraries/pkg/utils"

	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/panjf2000/ants/v2"
	"gitlab.com/daitheky/api-portal-admin/auth"
	"gitlab.com/daitheky/api-portal-admin/constant"
	"gitlab.com/daitheky/api-portal-admin/entity"
	"gitlab.com/daitheky/api-portal-admin/repository"
	"gitlab.com/daitheky/api-portal-admin/utils"
)

type CustomerHandlerImpl struct {
	repo       repository.CustomerRepository
	userRepo   repository.UserRepository
	deptRepo   repository.DeptRepository
	bikipRepo  repository.BikipRepository
	apiRepo    repository.ApiKeyRepository
	httpClient *resty.Client
}

func NewCustomerHandler(config *repository.Config) CustomerHandler {

	repo, err := repository.NewCustomerRepositoryImpl(config)
	if err != nil {
		return nil
	}

	userRepo, err := repository.NewUserRepositoryImpl(config)
	if err != nil {
		return nil
	}

	deptRepo, err := repository.NewDeptRepositoryImpl(config)
	if err != nil {
		return nil
	}

	bikipRepo, err := repository.NewBikipRepositoryImpl(config)
	if err != nil {
		return nil
	}

	apiRepo, err := repository.NewApiKeyRepositoryImpl(config)
	if err != nil {
		return nil
	}

	return &CustomerHandlerImpl{
		repo:       repo,
		userRepo:   userRepo,
		deptRepo:   deptRepo,
		bikipRepo:  bikipRepo,
		apiRepo:    apiRepo,
		httpClient: resty.New(),
	}
}

func (s *CustomerHandlerImpl) bind(c echo.Context, pointer interface{}) error {
	if err := c.Bind(pointer); err != nil {
		return err
	}
	if err := c.Validate(pointer); err != nil {
		return err
	}
	return nil
}

type QueryCustomer struct {
	Keyword   string    `param:"keyword" query:"keyword" form:"keyword" json:"keyword"`
	Districts []string  `param:"districts" query:"districts" form:"districts" json:"districts"`
	Budget    []float32 `param:"budget" query:"budget" form:"budget" json:"budget"`
	Status    *int      `param:"status" query:"status" form:"status" json:"status"`
	Dept      string    `param:"dept" query:"dept" form:"dept" json:"dept"`
	User      string    `param:"user" query:"user" form:"user" json:"user"`
	Offset    int       `param:"offset" query:"offset" form:"offset" json:"offset"`
	Limit     int       `param:"limit" query:"limit" form:"limit" json:"limit"`
	Sort      string    `param:"sort" query:"sort" form:"sort" json:"sort"`
}

func (s *CustomerHandlerImpl) List(c echo.Context) error {

	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	// if userInfo.Group >= constant.GroupChuyenGia {
	// 	return c.JSON(http.StatusBadRequest, Response{
	// 		Code:    http.StatusBadRequest,
	// 		Message: "Permission denied",
	// 	})
	// }

	var request QueryCustomer
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	query := make(map[string]map[string]interface{})

	// if len(request.Value) > 0 {
	// 	if err := json.Unmarshal([]byte(request.Value), &query); err != nil {
	// 		return c.JSON(http.StatusBadRequest, Response{
	// 			Code:    http.StatusBadRequest,
	// 			Message: "Invalid params",
	// 		})
	// 	}
	// }

	if len(request.Keyword) > 0 {
		query["keyword"] = map[string]interface{}{
			"type":  "wildcard",
			"value": request.Keyword,
		}
	}
	if request.Status != nil {
		query["status"] = map[string]interface{}{
			"type":  "term",
			"value": strconv.Itoa(*request.Status),
		}
	}
	if request.Status != nil {
		query["status"] = map[string]interface{}{
			"type":  "term",
			"value": strconv.Itoa(*request.Status),
		}
	}
	if request.Districts != nil && len(request.Districts) > 0 {
		query["districts"] = map[string]interface{}{
			"type":  "terms",
			"value": request.Districts,
		}
	}
	if request.Budget != nil && len(request.Budget) > 0 {
		query["budget"] = map[string]interface{}{
			"type":  "range",
			"value": request.Budget,
		}
	}
	if len(request.Dept) > 0 {
		query["dept_id"] = map[string]interface{}{
			"type":  "term",
			"value": request.Dept,
		}
	}
	if len(request.User) > 0 {
		query["user_id"] = map[string]interface{}{
			"type":  "term",
			"value": request.User,
		}
	}

	canViewMember := cutils.Contains(userInfo.Perms, constant.PermMemberView) || cutils.Contains(userInfo.Perms, constant.PermAdminMemberView)
	if len(request.User) > 0 && canViewMember {
		query["user_id"] = map[string]interface{}{
			"type":  "term",
			"value": request.User,
		}
	}
	if !canViewMember {
		query["user_id"] = map[string]interface{}{
			"type":  "term",
			"value": userInfo.ID,
		}
	}
	if !cutils.Contains(userInfo.Perms, constant.PermAdminMemberView) {
		query["dept_id"] = map[string]interface{}{
			"type":  "term",
			"value": userInfo.Dept,
		}
	}

	count, err := s.repo.Count(query)
	if err != nil {
		return c.JSON(http.StatusOK, Response{
			Code:    http.StatusBadRequest,
			Message: "Not found",
			Data:    err.Error(),
		})
	}

	if count == 0 {
		return c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "Success",
			Data: map[string]interface{}{
				"items": []entity.Customer{},
				"total": 0,
			},
		})
	}

	sortField := request.Sort
	if sortField == "created" {
		sortField = "created_at"
	} else {
		sortField = "lead_at"
	}

	customers, _, err := s.repo.List(query, sortField, request.Offset, request.Limit)
	if err != nil {
		return c.JSON(http.StatusOK, Response{
			Code:    http.StatusBadRequest,
			Message: "Not found",
			Data:    err.Error(),
		})
	}

	var wg sync.WaitGroup
	p, _ := ants.NewPoolWithFunc(6, func(i interface{}) {
		cus := i.(*entity.Customer)
		u, err := s.userRepo.GetByID(cus.UserID)
		if err != nil {
			return
		}
		cusUser := &entity.User{
			ID:       u.ID,
			FullName: u.FullName,
			DeptName: u.DeptName,
			Phone:    u.Phone,
		}
		cus.User = cusUser

		leads, count, err := s.repo.ListLead(cus.ID, 0, 10)
		if leads != nil && err == nil {
			for i := 0; i < len(leads); i++ {
				leads[i].User = cusUser
				if len(leads[i].BikipID) > 0 {
					bikip, err := s.bikipRepo.Get(leads[i].BikipID)
					if err != nil {
						continue
					}
					leads[i].Bikip = &entity.Bikip{
						ID:    bikip.ID,
						Title: bikip.Title,
					}
				}
			}
			cus.Lead = &entity.ConfidenceLead{
				Items: leads,
				Total: count,
			}
		}
		wg.Done()
	})

	defer p.Release()
	for i := 0; i < len(customers); i++ {
		wg.Add(1)
		_ = p.Invoke(customers[i])
	}

	wg.Wait()

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
		Data: map[string]interface{}{
			"items": customers,
			"total": count,
		},
	})
}

func (s *CustomerHandlerImpl) Add(c echo.Context) error {

	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	// if userInfo.Group >= constant.GroupChuyenGia {
	// 	return c.JSON(http.StatusBadRequest, Response{
	// 		Code:    http.StatusBadRequest,
	// 		Message: "Permission denied",
	// 	})
	// }

	var param entity.CustomerParam
	err := s.bind(c, &param)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
			Data:    err.Error(),
		})
	}

	// Create album image
	apiKey, err := s.apiRepo.GetBy("user_id", userInfo.ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusUnprocessableEntity,
			Message: "get api key error",
		})
	}

	space := regexp.MustCompile(`\s+`)
	fullName := strings.Trim(param.FullName, " ,.")
	// fullName = space.ReplaceAllString(fullName, " ")

	lastCMND := strings.Trim(param.LastCMND, " ,.")
	lastCMND = space.ReplaceAllString(lastCMND, " ")

	lastPhone := strings.Trim(param.Phone, " ,.")
	lastPhone = space.ReplaceAllString(lastPhone, " ")

	note := strings.Trim(param.Note, " ")

	// customerId := hash.MD5(strings.ToLower(fullName + lastCMND))
	customerId := uuid.New().String()

	existedCustomer, err := s.repo.GetByID(customerId)
	if err == nil && existedCustomer != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Khách hàng đã tồn tại!",
		})
	}

	currentTime := time.Now()
	currentTime = currentTime.Round(time.Second)

	candidate := &entity.Customer{
		ID:        customerId,
		FullName:  fullName,
		BirthYear: param.BirthYear,
		City:      param.City,
		Address:   param.Address,
		LastCMND:  lastCMND,
		Phone:     lastPhone,
		Budget:    param.Budget,
		Note:      note,

		Zone:     userInfo.Zone,
		Province: userInfo.City,

		Districts: param.Districts,

		UserID: userInfo.ID,
		DeptID: userInfo.Dept,

		CreatedAt: currentTime,
		UpdatedAt: currentTime,
	}

	if len(param.Province) > 0 && userInfo.Group <= constant.GroupQTV {
		candidate.Province = param.Province
	}

	// Set album id
	albumId, err := s.createAlbum(apiKey.ID, "KH - "+fullName, fullName)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
	}

	candidate.Album = albumId

	leads := make([]*entity.CustomerLead, 0)
	if param.Leads != nil && len(param.Leads) > 0 {
		for _, l := range param.Leads {
			lead := &entity.CustomerLead{
				ID:        uuid.New().String(),
				CID:       customerId,
				UserID:    userInfo.ID,
				BikipID:   l.Bikip.ID,
				Comment:   l.Comment,
				Images:    l.Images,
				RegAt:     l.RegAt,
				CreatedAt: currentTime,
				UpdatedAt: currentTime,
			}

			var albumName = l.Bikip.Title
			indexHD := strings.Index(strings.ToLower(albumName), "tỷ")
			if indexHD > -1 {
				albumName = albumName[:indexHD] + "tỷ"
			}

			if len(albumName) > 90 {
				albumName = albumName[:90]
			}
			albumName = "LEAD - " + albumName
			leadAlbumId, err := utils.CreateAlbum(apiKey.ID, albumName, l.Bikip.Title, candidate.Album)
			if err != nil {
				log.Println(err)
				continue
			}

			var imgIds = make([]string, 0)
			for idx, img := range lead.Images {
				imgIds = append(imgIds, lead.Images[idx].GalleryID)
				img.Album = leadAlbumId
				img.Category = "7" // Category customer
			}

			if len(imgIds) > 0 {
				_, err = s.editPhotoToAlbum(apiKey.ID, imgIds, leadAlbumId, "", "", false)
				if err != nil {
					return c.JSON(http.StatusBadRequest, Response{
						Code:    http.StatusUnprocessableEntity,
						Message: "lỗi upload ảnh: " + err.Error(),
					})
				}
			}

			leads = append(leads, lead)
			if candidate.LeadAt == nil || candidate.LeadAt.Unix() < l.RegAt.Unix() {
				candidate.LeadAt = &l.RegAt
			}
		}
	}

	err = s.repo.Create(candidate)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	if len(leads) > 0 {
		err = s.repo.AddLead(leads)
		if err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: "Add customer lead error",
			})
		}
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
	})
}

func (h *CustomerHandlerImpl) editPhotoToAlbum(apiKey string, photoIds []string, albumId string, albumName string, albumDesc string, newAlbum bool) (string, error) {
	h.httpClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	h.httpClient.SetTimeout(time.Second * 10)
	data := map[string]string{
		"key":                  apiKey,
		"editing[album_id]":    albumId,
		"editing[category_id]": "7", // 2 category tuan123
		"edit":                 "image",
	}

	if newAlbum {
		data["editing[album_name]"] = albumName
		data["editing[album_privacy]"] = "password"
		data["editing[album_description]"] = albumDesc
		data["editing[album_password]"] = "6705a3bb63e4"
		data["editing[new_album]"] = strconv.FormatBool(newAlbum)
	}

	imageIds := url.Values{
		"editing[ids][]": photoIds,
	}

	resp, err := h.httpClient.R().
		SetFormData(data).
		SetFormDataFromValues(imageIds).
		Post("https://api-dtk.thangbk.com/photos/edit")

	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", errors.New(string(resp.Body()))
	}

	var res = make(map[string]interface{})
	err = json.Unmarshal(resp.Body(), &res)
	if err != nil {
		return "", err
	}

	if newAlbum {
		album := res["album"].(map[string]interface{})
		newAlbumId := album["id_encoded"].(string)
		return newAlbumId, nil
	}

	return albumId, nil
}

func (h *CustomerHandlerImpl) createAlbum(apiKey, name, desc string) (string, error) {

	h.httpClient.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	h.httpClient.SetTimeout(time.Second * 10)
	data := map[string]string{
		"key":                apiKey,
		"type":               "album",
		"album[name]":        name,
		"album[description]": desc,
		"album[privacy]":     "password",
		"album[password]":    "6705a3bb63e4",
		"album[new]":         "true",
	}

	resp, err := h.httpClient.R().
		SetFormData(data).
		Post("https://album.daitheky.net/api/1/create-album")

	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", errors.New("cannot create album")
	}

	var res = make(map[string]interface{})
	err = json.Unmarshal(resp.Body(), &res)
	if err != nil {
		return "", err
	}

	album := res["album"].(map[string]interface{})
	if albumId, ok := album["id_encoded"].(string); ok {
		return albumId, nil
	}

	return "", nil
}

func (s *CustomerHandlerImpl) Info(c echo.Context) error {

	// userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	// if userInfo.Group >= constant.GroupChuyenGia {
	// 	return c.JSON(http.StatusBadRequest, Response{
	// 		Code:    http.StatusBadRequest,
	// 		Message: "Permission denied",
	// 	})
	// }

	id := c.Param("id")
	candidate, err := s.repo.GetByID(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Không tìm thấy người dùng với ID: " + id,
		})
	}

	user, _ := s.userRepo.GetByID(candidate.UserID)

	candidate.User = user

	userDept, _ := s.deptRepo.GetByID(candidate.DeptID)
	candidate.Dept = userDept

	leads, count, err := s.repo.ListLead(id, 0, 10)
	if leads != nil && err == nil {
		for i := 0; i < len(leads); i++ {
			u, err := s.userRepo.GetByID(leads[i].UserID)
			if err != nil {
				continue
			}
			leads[i].User = &entity.User{
				ID:       leads[i].UserID,
				FullName: u.FullName,
				DeptName: u.DeptName,
				Phone:    u.Phone,
			}

			if len(leads[i].BikipID) > 0 {
				bikip, err := s.bikipRepo.Get(leads[i].BikipID)
				if err != nil {
					continue
				}
				leads[i].Bikip = &entity.Bikip{
					ID:    bikip.ID,
					Title: bikip.Title,
				}
			}
		}
		candidate.Lead = &entity.ConfidenceLead{
			Items: leads,
			Total: count,
		}
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
		Data:    candidate,
	})
}

func (s *CustomerHandlerImpl) UpdateStatus(c echo.Context) error {
	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	id := c.Param("id")

	var param entity.CustomerParam
	err := c.Bind(&param)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
			Data:    err.Error(),
		})
	}

	// status := c.QueryParam("status")
	// if len(status) == 0 {
	// 	return c.JSON(http.StatusBadRequest, Response{
	// 		Code:    http.StatusBadRequest,
	// 		Message: "Invalid params",
	// 	})
	// }

	existedCustomer, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// existedCandidate.UserID = userId
	if existedCustomer.UserID != userInfo.ID {
		return c.JSON(http.StatusForbidden, Response{
			Code:    http.StatusForbidden,
			Message: "permission denied: you are not the owner of customer",
		})
	}

	existedCustomer.Status = param.Status
	err = s.repo.Create(existedCustomer)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid params",
		})
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
	})
}

func (s *CustomerHandlerImpl) Update(c echo.Context) error {

	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	id := c.Param("id")
	var param entity.CustomerParam
	err := s.bind(c, &param)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	existedCustomer, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if existedCustomer.UserID != userInfo.ID {
		return c.JSON(http.StatusForbidden, Response{
			Code:    http.StatusForbidden,
			Message: "permission denied: you are not the owner of customer",
		})
	}

	space := regexp.MustCompile(`\s+`)
	// fullName := strings.Trim(param.FullName, " ,.")
	// fullName = space.ReplaceAllString(fullName, " ")

	lastCMND := strings.Trim(param.LastCMND, " ,.")
	lastCMND = space.ReplaceAllString(lastCMND, " ")

	lastPhone := strings.Trim(param.Phone, " ,.")
	lastPhone = space.ReplaceAllString(lastPhone, " ")

	currentTime := time.Now()
	currentTime = currentTime.Round(time.Second)

	existedCustomer.BirthYear = param.BirthYear
	existedCustomer.City = param.City
	existedCustomer.LastCMND = lastCMND
	existedCustomer.Phone = lastPhone
	existedCustomer.Budget = param.Budget
	existedCustomer.Districts = param.Districts
	existedCustomer.UpdatedAt = currentTime

	err = s.repo.Create(existedCustomer)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, Response{
			Code:    http.StatusUnprocessableEntity,
			Message: "Invalid params",
		})
	}

	// leads := existedCustomer.Leads
	// if leads == nil {
	// 	leads = make([]*entity.CustomerLead, 0)
	// }

	leads := make([]*entity.CustomerLead, 0)

	if param.Leads != nil && len(param.Leads) > 0 {
		for _, l := range param.Leads {
			if len(l.ID) != 0 {
				continue
			}

			lead := &entity.CustomerLead{
				ID:        uuid.New().String(),
				CID:       existedCustomer.ID,
				UserID:    userInfo.ID,
				BikipID:   l.BikipID,
				Comment:   l.Comment,
				Images:    l.Images,
				RegAt:     l.RegAt,
				CreatedAt: currentTime,
			}

			leads = append(leads, lead)
		}
	}

	err = s.repo.AddLead(leads)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "Cập nhật thông tin không thành công",
		})
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
	})
}

func (s *CustomerHandlerImpl) Lead(c echo.Context) error {
	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	// if userInfo.Group >= constant.GroupChuyenGia {
	// 	return c.JSON(http.StatusBadRequest, Response{
	// 		Code:    http.StatusBadRequest,
	// 		Message: "Permission denied",
	// 	})
	// }

	id := c.Param("id")
	candidate, err := s.repo.GetByID(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Không tìm thấy người dùng với ID: " + id,
		})
	}

	if candidate.UserID != userInfo.ID {
		return c.JSON(http.StatusForbidden, Response{
			Code:    http.StatusForbidden,
			Message: "permission denied: you are not the owner of customer",
		})
	}

	var params = make([]*entity.CustomerLeadParam, 0)
	// err = s.bind(c, &params)
	if err := (&echo.DefaultBinder{}).BindBody(c, &params); err != nil || len(params) == 0 {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	apiKey, err := s.apiRepo.GetBy("user_id", userInfo.ID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
			Data:    errors.New("get api key error"),
		})
	}

	currentTime := time.Now()
	leads := make([]*entity.CustomerLead, 0)
	for _, p := range params {
		lead := &entity.CustomerLead{
			ID:        uuid.New().String(),
			CID:       id,
			BikipID:   p.BikipID,
			UserID:    userInfo.ID,
			RegAt:     p.RegAt,
			Comment:   p.Comment,
			CreatedAt: currentTime,
		}

		bikip, err := s.bikipRepo.Get(p.BikipID)
		if err != nil {
			log.Println(err)
			continue
		}

		var albumName = bikip.Title
		indexHD := strings.Index(strings.ToLower(albumName), "tỷ")
		if indexHD > -1 {
			albumName = albumName[:indexHD] + "tỷ"
		}

		if len(albumName) > 90 {
			albumName = albumName[:90]
		}

		albumName = "LEAD - " + albumName
		albumId, err := utils.CreateAlbum(apiKey.ID, albumName, bikip.Title, candidate.Album)
		if err != nil {
			log.Println(err)
			continue
		}

		if p.Images != nil && len(p.Images) > 0 {
			for _, img := range p.Images {
				img.Album = albumId
			}
		}

		lead.Images = p.Images
		leads = append(leads, lead)
		if candidate.LeadAt == nil || candidate.LeadAt.Unix() < lead.RegAt.Unix() {
			candidate.LeadAt = &lead.RegAt
		}
	}

	err = s.repo.Create(candidate)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Lỗi cập nhật thông tin ID: " + id,
		})
	}
	if len(leads) > 0 {
		err = s.repo.AddLead(leads)
		if err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: "Add lead error",
				Data:    err.Error(),
			})
		}
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
	})
}

func (s *CustomerHandlerImpl) ListLead(c echo.Context) error {
	userInfo := c.Get(constant.KeyUserInfo).(*auth.Claims)
	if userInfo.Group >= constant.GroupChuyenGia {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Permission denied",
		})
	}

	id := c.Param("id")
	candidate, err := s.repo.GetByID(id)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Không tìm thấy người dùng với ID: " + id,
		})
	}

	if candidate.UserID != userInfo.ID {
		return c.JSON(http.StatusForbidden, Response{
			Code:    http.StatusForbidden,
			Message: "permission denied: you are not the owner of customer",
		})
	}

	var request QueryCustomer
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	leads, count, err := s.repo.ListLead(id, request.Offset, request.Limit)
	if err != nil {
		return c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "Invalid params",
		})
	}

	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
		Data: map[string]interface{}{
			"items": leads,
			"total": count,
		},
	})
}

func (s *CustomerHandlerImpl) UpdateLead(c echo.Context) error {
	return c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "Success",
	})
}

func (s *CustomerHandlerImpl) DeleteLead(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, Response{
		Code:    http.StatusBadRequest,
		Message: "Permission denied",
	})
}

func (s *CustomerHandlerImpl) Delete(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, Response{
		Code:    http.StatusBadRequest,
		Message: "Permission denied",
	})
}
