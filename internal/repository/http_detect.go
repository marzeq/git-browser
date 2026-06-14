package repository

import "net/http"

func httpDetectContentType(data []byte) string {
	return http.DetectContentType(data)
}
