from typing import Annotated

import jwt
from fastapi import HTTPException, Depends
from fastapi.security import OAuth2PasswordBearer
from jwt import InvalidTokenError
from pydantic import ValidationError
from sqlalchemy.orm import Session
from starlette import status

from backend.core import security
from backend.core.config import settings
from backend.db_models.db_session import AsyncSessionLocal
from backend.db_models.models import UserDB
from backend.repositories.users_repository import UsersRepository
from backend.schemas.user_schemas import TokenPayload

reusable_oauth2 = OAuth2PasswordBearer(
    tokenUrl=f"{settings.API_V1_PATH}/login/access-token"
)

async def get_session() -> Session:
    session = AsyncSessionLocal()
    try:
        yield session
    except Exception:
        await session.rollback()
        raise
    finally:
        await session.close()

TokenDep = Annotated[str, Depends(reusable_oauth2)]
DBSession = Annotated[Session, Depends(get_session)]

async def get_user(
    token: TokenDep,
    db_session: DBSession
) -> UserDB:
    try:
        payload = jwt.decode(
            token,
            settings.SECRET_KEY,
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

CurrentUser = Annotated[UserDB, Depends(get_user)]
