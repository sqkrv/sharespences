from backend.utils import DefaultException


class UserNotFoundException(DefaultException):
    default_message = "Пользователь не найден"
    error_code = "UserNotFound"


class AuthenticationError(DefaultException):
    default_message = "Неверный логин или пароль"
    error_code = "AuthenticationError"

