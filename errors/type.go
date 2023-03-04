package errors

func BadRequest(msg, reason string) *Error {
	return New(RequestBadCode, msg, reason)
}

func ServerError(msg, reason string) *Error {
	return New(InternalServerCode, msg, reason)
}

func Unauthorized(msg, reason string) *Error {
	return New(DeniedCode, msg, reason)
}

func ParamInvalid(msg, reason string) *Error {
	return New(ParamValidCode, msg, reason)
}

func FormatInvalid(msg, reason string) *Error {
	return New(FormatInvalidCode, msg, reason)
}

func DatabaseError(msg, reason string) *Error {
	return New(DatabaseCode, msg, reason)
}

func TokenInvalid(msg, reason string) *Error {
	return New(InvalidTokenCode, msg, reason)
}

func TokenExpire(msg, reason string) *Error {
	return New(ExpireTokenCode, msg, reason)
}
