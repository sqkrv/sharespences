import logging

from sqlalchemy import select
from sqlalchemy.orm import Session, selectinload

from backend.db_models.models.other_models import CategoryDB, BankDB, MCCCodeDB

logger = logging.getLogger(__name__)


class CommonRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def get_banks(self) -> list[BankDB]:
        stmt = (select(BankDB)
                .order_by(BankDB.id))
        query = await self._db_session.execute(stmt)
        return query.scalars()

    async def get_categories_by_params(
            self,
            bank_id: int,
    ) -> list[CategoryDB]:
        stmt = (
            select(CategoryDB)
            .where(CategoryDB.bank_id == bank_id)
            .order_by(CategoryDB.id)
            .options(selectinload(CategoryDB.bank))
        )
        query = await self._db_session.execute(stmt)
        return query.scalars()

    async def get_mcc_codes(self) -> list[MCCCodeDB]:
        stmt = (select(MCCCodeDB)
                .order_by(MCCCodeDB.code))
        query = await self._db_session.execute(stmt)
        return query.scalars()

    async def get_mcc_code_by_code(
            self,
            code: int,
    ) -> MCCCodeDB | None:
        stmt = (select(MCCCodeDB)
                .where(MCCCodeDB.code == code))
        query = await self._db_session.execute(stmt)
        return query.scalar()
