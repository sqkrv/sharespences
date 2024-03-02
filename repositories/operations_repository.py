import datetime
import logging
from typing import List, Optional
from uuid import UUID

from sqlalchemy import update, select, text, insert, func, delete, extract, case
from sqlalchemy.orm import Session, selectinload

from db_models.models.other_models import OperationDB
from schemas.response_schemas import OperationResponseSchema

logger = logging.getLogger(__name__)


class OperationsRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def add_operations(
            self,
            operations: List[OperationDB],
    ):
        stmt = (insert(OperationDB),
                [operation.model_dump() for operation in operations]
                )
        query = await self._db_session.execute(*stmt)
        await self._db_session.commit()

    async def get_operations_by_params(
            self,
            # article_type: ArticleType = None,
            query: str = None) -> List[OperationDB]:
        stmt = (select(OperationDB))
                # .order_by(OperationDB.published_at.desc())
                # .options(selectinload(ArticleDB.attachments)))
        if query is not None:
            stmt = stmt.where(OperationDB.title.ilike(f"%{query}%"))
        query = await self._db_session.execute(stmt)
        return query.scalars()

    async def get_operation_by_id(
            self,
            operation_id: int) -> OperationDB | None:
        stmt = (select(OperationDB)
                .where(OperationDB.id == operation_id)
                .options(selectinload(OperationDB.attachments)))
        query = await self._db_session.execute(stmt)
        return query.scalar()
