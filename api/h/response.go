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

// MessageItemsResponse 同时返回提示信息与实体列表。
type MessageItemsResponse[T any] struct {
	Status int `json:"-"`
	Body   struct {
		Message string `json:"message" doc:"提示信息"`
		Items   []T    `json:"items" doc:"响应列表"`
	} `json:"body"`
}

// NewMessageItemsResponse 构造消息 + 实体列表返回。
func NewMessageItemsResponse[T any](message string, items []T) *MessageItemsResponse[T] {
	resp := &MessageItemsResponse[T]{}
	resp.Body.Message = message
	resp.Body.Items = items
	return resp
}
