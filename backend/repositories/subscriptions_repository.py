import logging
from collections.abc import Sequence

from sqlalchemy.orm import selectinload
from sqlmodel import select
from sqlmodel.ext.asyncio.session import AsyncSession

from backend.models.common_models import BankCard
from backend.models.subscriptions_models import Subscription

logger = logging.getLogger(__name__)


class SubscriptionsRepository:
    def __init__(self, db_session: AsyncSession):
        self._db_session = db_session

    async def get_subscriptions(self) -> Sequence[Subscription]:
        stmt = (select(Subscription)
                .order_by(Subscription.next_payment_date)
                .options(selectinload(Subscription.members)))
        results = await self._db_session.exec(stmt)
        return results.all()
