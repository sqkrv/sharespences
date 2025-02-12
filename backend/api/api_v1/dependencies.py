from typing import Annotated, Any
from collections.abc import AsyncGenerator

import jwt
from fastapi import HTTPException, Depends
from fastapi.security import OAuth2PasswordBearer
from jwt import InvalidTokenError
from pydantic import ValidationError
from sqlmodel.ext.asyncio.session import AsyncSession
from starlette import status

from backend.core import security
from backend.core.config import settings
from backend.db.db_session import async_engine
from backend.models.auth_models import TokenPayload
from backend.models.users_models import User
from backend.repositories.users_repository import UsersRepository

reusable_oauth2 = OAuth2PasswordBearer(
    tokenUrl=f"{settings.API_V1_PATH}/auth/auth"  # doesn't work due to openapi not supporting webauthn
)

async def get_session() -> AsyncGenerator[AsyncSession, Any]:
        async with AsyncSession(bind=async_engine, expire_on_commit=False) as session:
            try:
                yield session
            except Exception:
                await session.rollback()
                raise

TokenDep = Annotated[str, Depends(reusable_oauth2)]
DBSession = Annotated[AsyncSession, Depends(get_session)]

async def get_user(
    token: TokenDep,
    db_session: DBSession
) -> User:
    try:
        payload = jwt.decode(
            token,
            settings.ACCESS_TOKEN_SECRET_KEY,
            algorithms=[security.ALGORITHM]
        )
        token_data = TokenPayload(**payload)
    except (InvalidTokenError, ValidationError):
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Could not validate credentials",
        )

    user_repository = UsersRepository(db_session)
    user = await user_repository.get_user_by_id(token_data.sub)
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    # if not user.is_active:
    #     raise HTTPException(status_code=400, detail="Inactive user")
    return user

CurrentUser = Annotated[User, Depends(get_user)]
