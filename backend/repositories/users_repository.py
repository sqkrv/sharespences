import logging
from uuid import UUID

from sqlalchemy import select, insert
from sqlalchemy.orm import Session, selectinload

from backend.db_models.models.user_models import UserDB

logger = logging.getLogger(__name__)


class UsersRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def create_user(
            self,
            user: UserDB
    ) -> UUID:
        # stmt = insert(UserDB).values()
        # await self._db_session.execute(stmt)
        self._db_session.add(user)
        await self._db_session.commit()
        await self._db_session.refresh(user)
        return user.id

    async def get_user_by_id(
            self,
            user_id: UUID
    ) -> UserDB | None:
        stmt = (select(UserDB)
                .where(UserDB.id == user_id))
        query = await self._db_session.execute(stmt)
        return query.scalar()
