import logging

from sqlalchemy import select
from sqlalchemy.orm import Session
from backend.db_models.models.user_models import PasskeyDB

logger = logging.getLogger(__name__)

class PasskeysRepository:
    def __init__(self, db_session: Session):
        self._db_session = db_session

    async def add_credential(
            self,
            passkey: PasskeyDB
    ) -> None:
        self._db_session.add(passkey)
        await self._db_session.commit()

    async def get_passkey(
            self,
            credential_id: str
    ) -> PasskeyDB | None:
        stmt = select(PasskeyDB).where(PasskeyDB.id == credential_id).limit(1)
        query = await self._db_session.execute(stmt)
        return query.scalar()