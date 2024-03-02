from typing import Annotated, Optional

import httpx
from fastapi import Header, HTTPException, Depends, Request
from fastapi.security import OAuth2PasswordBearer
from fastapi.security.utils import get_authorization_scheme_param
from pydantic import ValidationError
from httpx import BasicAuth
from sqlalchemy.orm import Session

import jwt
from starlette import status

from core import security
from core.config import settings
from core.constants.auth import INVALID_AUTHENTICATION_CREDENTIALS, USER_HAS_NO_PERMISSION
from db_models.db_session import AsyncSessionLocal
# from db_models.models.user_models import UserDB
# from engines.user_engines import UserEngine

# oauth2_scheme = OAuth2PasswordBearer(
#     tokenUrl=f'{settings.api_v1_path}/auth/login/'
# )


async def get_session() -> Session:
    session = AsyncSessionLocal()
    try:
        yield session
    except Exception:
        await session.rollback()
        raise
    finally:
        await session.close()

async def jwt_cookie_authentication(request: Request) -> Optional[str]:
    token = request.cookies.get(settings.TOKEN_NAME)
    # scheme, param = get_authorization_scheme_param(authorization)
    if not token:
        # if self.auto_error:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Not authenticated",
            headers={"WWW-Authenticate": "Cookie"},
        )
        # else:
        #     return None
    return token
