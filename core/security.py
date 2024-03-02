import datetime
from typing import List
from uuid import UUID

import jwt

from core.config import settings
# from db_models.models.user_models import RoleDB
# from schemas.response_schemas import RoleSchema

ALGORITHM = 'HS256'
# TOKEN_NAME = settings.TOKEN_NAME


def create_access_token(user_id: UUID, roles: List[...]) -> str:
    now = datetime.datetime.now(datetime.UTC)
    expire = now + datetime.timedelta(
        minutes=settings.access_token_expire_minutes
    )
    to_encode = {'exp': expire, 'sub': str(user_id), 'roles': [{"id": str(role.id), "name": role.title} for role in roles] if roles else None}
    encoded_jwt = jwt.encode(to_encode, settings.access_token_secret_key,
                             algorithm=ALGORITHM)
    return encoded_jwt


def create_refresh_token(user_id: UUID) -> str:
    expire = datetime.datetime.now(datetime.UTC) + datetime.timedelta(
        minutes=settings.refresh_token_expire_minutes
    )
    to_encode = {'exp': expire, 'sub': str(user_id)}
    encoded_jwt = jwt.encode(to_encode, settings.refresh_token_secret_key,
                             algorithm=ALGORITHM)
    return encoded_jwt
