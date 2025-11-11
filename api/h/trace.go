package h

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// HandlerInfo 存储真实handler的信息
type HandlerInfo struct {
	FuncName string
	FilePath string
	Line     int
}

// 全局handler映射表，key为路径+方法，value为真实handler信息
var (
	handlerMap = make(map[string]*HandlerInfo)
	mapMutex   sync.RWMutex
)

// HumaRegister 包装huma.Register，使用这种方式注册API，可以在日志中记录handler行号
func HumaRegister[I, O any](api huma.API, operation huma.Operation, handler func(context.Context, *I) (*O, error)) {
	huma.Register(HumaWrap(api, operation, handler))
}

func HumaTraceMiddleware(ctx huma.Context, next func(huma.Context)) {
	// 获取底层的 fiber context
	fiberCtx := humafiber.Unwrap(ctx)

	hInfo := GetHandlerInfo(ctx.Operation().OperationID)
	if hInfo != nil {
		fiberCtx.Locals("humaHandlerInfo", hInfo)
	}

	// 继续处理
	next(ctx)
}

// 第一个参数其实没有作用，但是不传的话 huma.Register(api, HumaWrap(operation, handler)) 会报错
func HumaWrap[I, O any](api huma.API, operation huma.Operation, handler func(context.Context, *I) (*O, error)) (huma.API, huma.Operation, func(context.Context, *I) (*O, error)) {
	// 获取handler的反射信息
	handlerValue := reflect.ValueOf(handler)
	if handlerValue.Kind() == reflect.Func {
		pc := handlerValue.Pointer()
		funcForPC := runtime.FuncForPC(pc)
		if funcForPC != nil {
			filePath, line := funcForPC.FileLine(pc)
			funcName := funcForPC.Name()

			wd, _ := os.Getwd()
			relPath, err := filepath.Rel(wd, filePath)
			if err != nil {
				return api, operation, handler
			}

			if operation.OperationID == "" {
				operation.OperationID, _ = gonanoid.New()
			}

			// 生成映射key
			key := operation.OperationID

			// 存储handler信息
			mapMutex.Lock()
			handlerMap[key] = &HandlerInfo{
				FuncName: funcName,
				FilePath: relPath,
				Line:     line,
			}
			mapMutex.Unlock()
		}
	}

	return api, operation, handler
}

// GetHandlerInfo 根据路径和方法获取真实handler信息
func GetHandlerInfo(operationID string) *HandlerInfo {
	mapMutex.RLock()
	defer mapMutex.RUnlock()
	return handlerMap[operationID]
}
