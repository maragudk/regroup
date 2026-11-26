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
	ErrorAlreadyMember      = Error("already a member")
	ErrorBodyEmpty          = Error("body empty")
	ErrorBodyInvalid        = Error("body invalid")
	ErrorBodyTooLong        = Error("body too long")
	ErrorCommentNotFound    = Error("comment not found")
	ErrorCursorInvalid      = Error("cursor invalid")
	ErrorEmailInvalid       = Error("email address invalid")
	ErrorFormerMember       = Error("former member")
	ErrorFormResentChanged  = Error("form sent again with other text")
	ErrorFormTokenInvalid   = Error("form token invalid")
	ErrorInvitationNotFound = Error("invitation not found")
	ErrorLinkInvalid        = Error("link invalid")
	ErrorMemberNotFound     = Error("member not found")
	ErrorNameEmpty          = Error("name empty")
	ErrorNameInvalid        = Error("name invalid")
	ErrorNameTooLong        = Error("name too long")
	ErrorNotAllowed         = Error("not allowed")
	ErrorPostNotFound       = Error("post not found")
	ErrorRoleInvalid        = Error("role invalid")
)
