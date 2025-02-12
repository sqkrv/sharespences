import logging

from fastapi import APIRouter

from backend.api.api_v1.dependencies import CurrentUser, DBSession
from backend.repositories.common_repository import CommonRepository
from backend.models.common_models import Bank, MCCCode, CategoryWithMCCCodes

router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "/banks",
    response_model=list[Bank],
)
async def banks_endpoint(
    _: CurrentUser,
    db_session: DBSession,
):
    common_repository = CommonRepository(db_session)
    banks = await common_repository.get_banks()
    return banks


@router.get(
    "/categories",
    response_model=list[CategoryWithMCCCodes],
)
async def categories_endpoint(
    bank_id: int,
    _: CurrentUser,
    db_session: DBSession,
):
    common_repository = CommonRepository(db_session)
    categories = await common_repository.get_categories_by_params(bank_id=bank_id)
    categories_with_mcc_codes = []
    for category in categories:
        category_with_mcc_codes = CategoryWithMCCCodes.model_validate(category, update={'mcc_codes': [mcc_code.code for mcc_code in category.mcc_codes]})
        categories_with_mcc_codes.append(category_with_mcc_codes)
    return categories_with_mcc_codes


@router.get(
    "/mcc_codes",
    response_model=list[MCCCode],
)
async def mcc_codes_endpoint(
    _: CurrentUser,
    db_session: DBSession,
):
    common_repository = CommonRepository(db_session)
    mcc_codes = await common_repository.get_mcc_codes()
    return mcc_codes


@router.get(
    "/mcc_codes/{code}",
    response_model=MCCCode | None,
)
async def mcc_code_endpoint(
    code: int,
    _: CurrentUser,
    db_session: DBSession,
):
    common_repository = CommonRepository(db_session)
    mcc_code = await common_repository.get_mcc_code_by_code(code=code)
    if mcc_code:
        return mcc_code

