import logging

from sqlalchemy import select, insert
from sqlalchemy.orm import Session, selectinload

from models import TransactionDB

logger = logging.getLogger(__name__)


class OperationsRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def add_operations(
            self,
            operations: list[TransactionDB],
    ):
        stmt = (insert(TransactionDB),
                [operation.model_dump() for operation in operations]
                )
        query = await self._db_session.execute(*stmt)
        await self._db_session.commit()

    async def get_operations_by_params(
            self,
            # article_type: ArticleType = None,
            query: str = None) -> list[TransactionDB]:
        stmt = (select(TransactionDB))
        # .order_by(TransactionDB.published_at.desc())
        # .options(selectinload(ArticleDB.attachments)))
        if query is not None:
            stmt = stmt.where(TransactionDB.title.ilike(f"%{query}%"))
        query = await self._db_session.execute(stmt)
        return query.scalars()

    async def get_operation_by_id(
            self,
            operation_id: int) -> TransactionDB | None:
        stmt = (select(TransactionDB)
                .where(TransactionDB.id == operation_id)
                .options(selectinload(TransactionDB.attachments)))
        query = await self._db_session.execute(stmt)
        return query.scalar()
