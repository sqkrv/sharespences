import datetime
from uuid import UUID

import jwt

from backend.core.config import settings

ALGORITHM = 'HS256'


# def create_access_token(user_id: UUID) -> str:
#     now = datetime.datetime.now(datetime.UTC)
#     expire = now + datetime.timedelta(
#         minutes=settings.access_token_expire_minutes
#     )
#     to_encode = {'exp': expire, 'sub': str(user_id)}
#     encoded_jwt = jwt.encode(to_encode, settings.access_token_secret_key,
#                              algorithm=ALGORITHM)
#     return encoded_jwt


def create_access_token(user_id: UUID) -> str:
    expire = datetime.datetime.now(datetime.UTC) + datetime.timedelta(minutes=settings.ACCESS_TOKEN_EXPIRE_MINUTES)
    to_encode = {"exp": expire, "sub": str(user_id)}
    encoded_jwt = jwt.encode(to_encode, settings.ACCESS_TOKEN_SECRET_KEY, algorithm=ALGORITHM)
    return encoded_jwt


def create_refresh_token(user_id: UUID) -> str:
    expire = datetime.datetime.now(datetime.UTC) + datetime.timedelta(minutes=settings.REFRESH_TOKEN_EXPIRE_MINUTES)
    to_encode = {'exp': expire, 'sub': str(user_id)}
    encoded_jwt = jwt.encode(to_encode, settings.REFRESH_TOKEN_SECRET_KEY, algorithm=ALGORITHM)
    return encoded_jwt
