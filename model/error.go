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
	ErrorBodyEmpty        = Error("body empty")
	ErrorBodyInvalid      = Error("body invalid")
	ErrorBodyTooLong      = Error("body too long")
	ErrorCommentNotFound  = Error("comment not found")
	ErrorCursorInvalid    = Error("cursor invalid")
	ErrorFormTokenInvalid = Error("form token invalid")
	ErrorPostNotFound     = Error("post not found")
)
