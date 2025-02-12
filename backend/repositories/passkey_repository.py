import logging

from sqlmodel.ext.asyncio.session import AsyncSession

from backend.models.users_models import Passkey

logger = logging.getLogger(__name__)

class PasskeysRepository:
    def __init__(self, db_session: AsyncSession):
        self._db_session = db_session

    async def add_credential(
            self,
            passkey: Passkey
    ) -> None:
        self._db_session.add(passkey)
        await self._db_session.commit()

    async def get_passkey(
            self,
            credential_id: str
    ) -> Passkey | None:
        return await self._db_session.get(Passkey, credential_id)
