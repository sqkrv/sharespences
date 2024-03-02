import datetime
from pathlib import Path
from typing import List, Annotated

from fastapi import APIRouter, Depends, Query, status, UploadFile, Form, File
import logging
from sqlalchemy.orm import Session
from starlette.responses import JSONResponse

from api.api_v1.dependencies import get_session
from core.config import settings
from repositories.operations_repository import OperationsRepository
from schemas.response_schemas import OperationResponseSchema

# from schemas.schema_enums.news_enums import ArticleType


router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "",
    # response_model=list[ArticleResponse]
)
async def get_operations_endpoint(
        # sort: List[Annotated[str, SortParam]] = Query(None,
        #                                               description="**Формат:** `[-]<название параметра>`\n\n**Пример URL:** `?sort=-number&sort=classifier`"),
        query: str = Query(None, description="Поддерживается поиск по названию и тексту статьи"),
        # user: UserDB = Depends(get_brigadier),
        db_session: Session = Depends(get_session)
):
    operations_repository = OperationsRepository(db_session)
    operations_db = await operations_repository.get_operations_by_params(query)
    return operations_db


@router.post(
    "",
    # response_model=ArticleResponse
)
async def add_operation_manually_endpoint(
        # article: ArticleSchema,
        attachments: Annotated[list[UploadFile], File(...)],
        # type: ArticleType = Form(...),
        title: str = Form(...),
        text: str = Form(...),
        db_session: Session = Depends(get_session)
):
    print(type, title, text, attachments[0].filename)

    operations_repository = OperationsRepository(db_session)
    await operations_repository.add_operations([])
