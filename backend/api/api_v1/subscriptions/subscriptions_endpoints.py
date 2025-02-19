import logging

from fastapi import APIRouter

from backend.api.api_v1.dependencies import CurrentUser, DBSession
from backend.repositories.subscriptions_repository import SubscriptionsRepository
from backend.models.subscriptions_models import Subscription, SubscriptionMember

router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "/",
    response_model=list[Subscription],
)
async def banks_endpoint(
    _: CurrentUser,
    db_session: DBSession,
):
    subscriptions_repository = SubscriptionsRepository(db_session)
    subscriptions = await subscriptions_repository.get_subscriptions()
    return subscriptions
