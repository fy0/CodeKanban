package api

import (
	"errors"

	"code-kanban/utils"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

func configStoreAPIError(err error, message string) error {
	if errors.Is(err, utils.ErrConfigStoreReadOnly) {
		return huma.Error503ServiceUnavailable(utils.ErrConfigStoreReadOnly.Error())
	}
	if errors.Is(err, utils.ErrConfigStoreNotInitialized) {
		return huma.Error503ServiceUnavailable("config store is not initialized")
	}
	return huma.Error500InternalServerError(message, err)
}

func sendConfigStoreFiberError(ctx *fiber.Ctx, err error, message string) error {
	if errors.Is(err, utils.ErrConfigStoreReadOnly) {
		return sendAPIError(ctx, 503, utils.ErrConfigStoreReadOnly.Error())
	}
	if errors.Is(err, utils.ErrConfigStoreNotInitialized) {
		return sendAPIError(ctx, 503, "config store is not initialized")
	}
	return sendAPIError(ctx, 500, message)
}
