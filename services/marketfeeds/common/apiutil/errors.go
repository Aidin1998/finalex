package apiutil

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/litebittech/cex/common/errors"
)

func ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		var statusCode errors.StatusCode
		if errors.As(err, &statusCode) {
			if statusCode < http.StatusInternalServerError {
				c.JSON(int(statusCode), err)
				return
			}

			c.NoContent(int(statusCode))
			return
		}

		var httpError *echo.HTTPError

		if errors.As(err, &httpError) {
			c.JSON(httpError.Code, httpError)
			return
		}

		c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"message": "internal server error",
		})
		return
	}
}
