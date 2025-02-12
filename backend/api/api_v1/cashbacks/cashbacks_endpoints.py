import datetime
import logging

from fastapi import APIRouter, Query

from backend.api.api_v1.dependencies import CurrentUser, DBSession
from backend.models.cashbacks_models import Cashback, CashbackAdd
from backend.repositories.cashbacks_repository import CashbacksRepository

router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "/",
    response_model=list[Cashback]
)
async def cashbacks_endpoint(
    relevance_date: datetime.date,
    user: CurrentUser,
    db_session: DBSession,
    bank_id: int = Query(None),
):
    cashbacks_repository = CashbacksRepository(db_session)
    cashbacks = await cashbacks_repository.get_cashbacks_by_params(
        user_id=user.id,
        relevance_date=relevance_date,
        bank_id=bank_id
    )
    return cashbacks

    user: CurrentUser,
    db_session: DBSession,
):
    cashbacks_repository = CashbacksRepository(db_session)
    cashbacks = await cashbacks_repository.get_cashbacks_by_params(user_id=user.id)
    return cashbacks
