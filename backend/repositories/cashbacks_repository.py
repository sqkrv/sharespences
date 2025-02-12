import datetime
import logging
from uuid import UUID

from sqlalchemy.orm import selectinload
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from backend.models.cashbacks_models import Cashback
from backend.models.common_models import Category

logger = logging.getLogger(__name__)


class CashbacksRepository:
    def __init__(self, db_session: AsyncSession):
        self._db_session = db_session

    async def get_cashbacks_by_params(
            self,
            user_id: UUID,
            relevance_date: datetime.date,
            bank_id: int = None,
    ) -> list[Cashback]:
        stmt = (
            select(Cashback)
            .where(Cashback.user_id == user_id)
            .where(Cashback.start_date <= relevance_date)
            .where(Cashback.end_date >= relevance_date)
            .order_by(Cashback.id)
            .options(selectinload(Cashback.category),
                     selectinload(Cashback.category, Category.bank))
        )
        if bank_id is not None:
            stmt = stmt.where(Cashback.category.has(Category.bank_id == bank_id))

        query = await self._db_session.exec(stmt)
        return query.all()
