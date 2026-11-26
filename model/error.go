package model

import "maragu.dev/glue/model"

const (
	ErrorEmailConflict = model.ErrorEmailConflict
	ErrorTokenExpired  = model.ErrorTokenExpired
	ErrorTokenNotFound = model.ErrorTokenNotFound
	ErrorUserInactive  = model.ErrorUserInactive
	ErrorUserNotFound  = model.ErrorUserNotFound
)

const (
	ErrorBodyEmpty     = Error("body empty")
	ErrorBodyInvalid   = Error("body invalid")
	ErrorBodyTooLong   = Error("body too long")
	ErrorCursorInvalid = Error("cursor invalid")
	ErrorPostNotFound  = Error("post not found")
)
