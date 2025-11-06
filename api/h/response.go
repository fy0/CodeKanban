package h

// ItemResponse 用于封装单实体返回结构。
type ItemResponse[T any] struct {
	Status int `json:"-"`
	Body   struct {
		Item T `json:"item" doc:"响应对象"`
	} `json:"body"`
}

// NewItemResponse 构造单实体响应。
func NewItemResponse[T any](item T) *ItemResponse[T] {
	resp := &ItemResponse[T]{}
	resp.Body.Item = item
	return resp
}

// ItemsResponse 用于封装列表返回结构。
type ItemsResponse[T any] struct {
	Status int `json:"-"`
	Body   struct {
		Items []T `json:"items" doc:"响应列表"`
	} `json:"body"`
}

// NewItemsResponse 构造列表响应。
func NewItemsResponse[T any](items []T) *ItemsResponse[T] {
	resp := &ItemsResponse[T]{}
	resp.Body.Items = items
	return resp
}

// MessageResponse 封装通用消息返回结构。
type MessageResponse struct {
	Status int `json:"-"`
	Body   struct {
		Message string `json:"message" doc:"提示信息"`
	} `json:"body"`
}

// NewMessageResponse 构造通用消息返回。
func NewMessageResponse(message string) *MessageResponse {
	resp := &MessageResponse{}
	resp.Body.Message = message
	return resp
}

// MessageItemResponse 同时返回提示信息与单实体。
type MessageItemResponse[T any] struct {
	Status int `json:"-"`
	Body   struct {
		Message string `json:"message" doc:"提示信息"`
		Item    T      `json:"item" doc:"响应对象"`
	} `json:"body"`
}

// NewMessageItemResponse 构造消息 + 单实体返回。
func NewMessageItemResponse[T any](message string, item T) *MessageItemResponse[T] {
	resp := &MessageItemResponse[T]{}
	resp.Body.Message = message
	resp.Body.Item = item
	return resp
}

// PaginatedResponse 通用分页返回结构。
type PaginatedResponse[T any] struct {
	Status int `json:"-"`
	Body   struct {
		Items     []T   `json:"items" doc:"响应列表"`
		Total     int64 `json:"total" doc:"记录总数"`
		ItemCount int64 `json:"itemCount" doc:"本次返回数量"`
		Page      int   `json:"page" doc:"当前页码"`
		PageSize  int   `json:"pageSize" doc:"每页数量"`
	} `json:"body"`
}

// NewPaginatedResponse 构造分页返回结构。
func NewPaginatedResponse[T any](items []T, total int64, page, pageSize int) *PaginatedResponse[T] {
	resp := &PaginatedResponse[T]{}
	resp.Body.Items = items
	resp.Body.Total = total
	resp.Body.ItemCount = int64(len(items))
	resp.Body.Page = page
	resp.Body.PageSize = pageSize
	return resp
}

// RowsAffectedResponse 通用受影响行数响应
type RowsAffectedResponse struct {
	Status int `json:"-"`
	Body   struct {
		RowsAffected int64 `json:"rowsAffected" doc:"受影响行数"`
	} `json:"body"`
}

// NewRowsAffectedResponse 构建受影响行数响应
func NewRowsAffectedResponse(rowsAffected int64) *RowsAffectedResponse {
	return &RowsAffectedResponse{
		Body: struct {
			RowsAffected int64 `json:"rowsAffected" doc:"受影响行数"`
		}{
			RowsAffected: rowsAffected,
		},
	}
}
