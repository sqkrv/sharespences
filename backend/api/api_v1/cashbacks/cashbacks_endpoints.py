from typing import List

from fastapi import APIRouter
import logging

from backend.api.api_v1.dependencies import CurrentUser, DBSession

from backend.schemas.response_schemas import Cashback
from repositories.cashbacks_repository import CashbacksRepository

router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "/",
    response_model=List[Cashback]
)
async def cashbacks_endpoint(
    user: CurrentUser,
    db_session: DBSession,
):
    cashbacks_repository = CashbacksRepository(db_session)
    cashbacks = await cashbacks_repository.get_cashbacks_by_params(user_id=user.id)
    return cashbacks
