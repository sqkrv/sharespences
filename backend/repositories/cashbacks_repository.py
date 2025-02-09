import logging
from typing import List
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session, selectinload

from backend.db_models.models.other_models import TransactionDB, CategoryDB
from db_models.models.cashbacks_models import CashbackDB

logger = logging.getLogger(__name__)


class CashbacksRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def get_cashbacks_by_params(
            self,
            user_id: UUID,
    ) -> List[TransactionDB]:
        stmt = (
            select(CashbackDB)
            .where(CashbackDB.user_id == user_id)
            # .order_by(TransactionDB.published_at.desc())
            .options(selectinload(CashbackDB.category),
                     selectinload(CashbackDB.category, CategoryDB.bank))
        )
        query = await self._db_session.execute(stmt)
        return query.scalars()

