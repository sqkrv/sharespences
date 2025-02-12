import logging
from collections.abc import Sequence

from sqlalchemy.orm import selectinload
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from backend.models.common_models import Category, Bank, MCCCode

logger = logging.getLogger(__name__)


class CommonRepository:
    def __init__(self, db_session: AsyncSession):
        self._db_session = db_session

    async def get_banks(self) -> Sequence[Bank]:
        stmt = (select(Bank)
                .order_by(Bank.id))
        results = await self._db_session.exec(stmt)
        return results.all()

    async def get_categories_by_params(
            self,
            bank_id: int,
    ) -> Sequence[Category]:
        stmt = (
            select(Category)
            .where(Category.bank_id == bank_id)
            .order_by(Category.id)
            .options(selectinload(Category.bank),
                     selectinload(Category.mcc_codes))
        )
        results = await self._db_session.exec(stmt)
        return results.all()

    async def get_mcc_codes(self) -> Sequence[MCCCode]:
        stmt = (select(MCCCode)
                .order_by(MCCCode.code))
        query = await self._db_session.exec(stmt)
        return query.all()

    async def get_mcc_code_by_code(
            self,
            code: int,
    ) -> MCCCode | None:
        return await self._db_session.get(MCCCode, code)
