package service

func logError(message string, err error, fields ...interface{}) {
	if err == nil {
		Logger.Errorw(message, fields...)
		return
	}
	Logger.Errorw(message, append(fields, "error", err)...)
}
