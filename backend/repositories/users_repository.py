import logging
from uuid import UUID

from sqlmodel.ext.asyncio.session import AsyncSession

from backend.models.users_models import User

logger = logging.getLogger(__name__)


class UsersRepository:
    def __init__(self, db_session: AsyncSession):
        self._db_session = db_session

    async def create_user(
            self,
            user: User
    ) -> UUID:
        self._db_session.add(user)
        await self._db_session.commit()
        await self._db_session.refresh(user)
        return user.id

    async def get_user_by_id(
            self,
            user_id: UUID
    ) -> User | None:
        return await self._db_session.get(User, user_id)
