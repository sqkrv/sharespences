from uuid import UUID

from fastapi import APIRouter, Depends, Query, status
from fastapi.responses import JSONResponse
import logging
from sqlalchemy.orm import Session

from backend.api.api_v1.dependencies import get_session, get_user
from backend.core.config import settings
from models import UserDB
# from backend.schemas.common_schemas import SortParam

from backend.repositories.operations_repository import OperationsRepository
from backend.schemas.response_schemas import (
    Operation,
    GenericResponse, MinimalOperation,
)


router = APIRouter()
logger = logging.getLogger(__name__)


@router.get(
    "/",
    response_model=list[MinimalOperation]
)
async def tasks_endpoint(
    # sort: List[Annotated[str, SortParam]] = Query(
    #     None,
    #     description="**Формат:** `[-]<название параметра>`\n\n**Пример URL:** `?sort=-number&sort=classifier`",
    # ),
    # status_filter: TaskStatusEnum = Query(None, description="Фильтрация по статусу"),
    production_block_filter: UUID = Query(None, description="Фильтрация по блоку"),
    query: str = Query(
        None, description="Поддерживается поиск по рядам и классификаторам"
    ),
    user: UserDB = Depends(get_user),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = OperationsRepository(db_session)
    operations = await brigadier_repository.get_operations_by_params(
        # sort=sort,
        # status_filter=status_filter,
        # production_block_filter=production_block_filter,
        query=query,
    )
    return operations


@router.get(
    "/{operation_id}",
    response_model=Operation
)
async def get_task_endpoint(
    operation_id: UUID,
    user_db: UserDB = Depends(get_user),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    full_operation_db = await brigadier_repository.get_full_task_by_id(operation_id)
    if full_operation_db:
        return full_operation_db
    return JSONResponse(GenericResponse(result="Task not found"), status_code=status.HTTP_404_NOT_FOUND)






@router.delete(
    "/tasks/{task_id}",
    deprecated=True
)
async def delete_task_endpoint(
    task_id: UUID,
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    if await brigadier_repository.delete_task(task_id):
        return JSONResponse({"detail": "OK"})
    return JSONResponse({"detail": "Not found"})


@router.post("/tasks/{task_id}/cancel")
async def cancel_task_endpoint(
    task_id: UUID,
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    return await cancel_task_usecase(db_session, task_id)


@router.post("/tasks/{task_id}/workers_rows")
async def workers_rows_task_endpoint(
    task_id: UUID,
    workers_rows_map: list[WorkerRowMapSchema],
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    if workers_rows_map:
        brigadier_repository = BrigadierRepository(db_session)
        task_db = await brigadier_repository.get_full_task_by_id(task_id)
        if not task_db.classifier.arm.is_duplicable:
            return JSONResponse(
                {"detail": "Task is not duplicable"},
                status_code=status.HTTP_400_BAD_REQUEST,
            )
        return await brigadier_repository.set_workers_rows_task(
            task_db, workers_rows_map
        )


@router.patch(
    "/tasks/{task_id}/progress",
    response_model=Task,
    description="Указанная задача обязана находиться в подтверждении (`Task.status` == `TaskStatusEnum.in_confirmation`)",
)
async def modify_progress_task_endpoint(
    task_id: UUID,
    task_progress_patch: TaskProgressPatchSchema,
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    task_db = await brigadier_repository.get_task_by_id(task_id)
    if task_db.status != TaskStatusEnum.in_confirmation:
        return JSONResponse(
            {"detail": "Данная задача не находится на подтверждении"},
            status_code=status.HTTP_400_BAD_REQUEST,
        )
    return await brigadier_repository.patch_task_progress(task_id, task_progress_patch)


@router.post(
    "/tasks",
    response_model=List[UUID]
)
async def add_task_endpoint(
    task_data: TaskCreateSchema,
    brigadier: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    tasks_db = await brigadier_repository.add_task(task_data, brigadier.id)
    return tasks_db


@router.patch(
    "/tasks/{task_id}",
    response_model=Task
)
async def patch_task_endpoint(
    task_id: UUID,
    task_patch: TaskPatchSchema,
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
    http_client=Depends(get_http_client),
):
    return await patch_task_usecase(task_id, task_patch, db_session, http_client)


@router.get("/classifiers", response_model=List[ClassifierResponse])
async def classifiers_endpoint(
    brigadier: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    classifiers = await brigadier_repository.get_all_classifiers(brigadier)
    return classifiers


@router.get("/verification/key", response_model=BrigadierVerificationKeyResponse)
async def get_verification_key_endpoint(_: UserDB = Depends(get_brigadier)):
    return BrigadierVerificationKeyResponse(
        key=settings.brigadier_verification_key, ttl=settings.brigadier_verification_ttl
    )


@router.get("/workers", response_model=List[VerifiedWorkerViewResponse])
async def get_verified_workers_endpoint(
    production_block_id: UUID = None,
    brigadier: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    verified_workers_db = await brigadier_repository.get_verified_workers(
        brigadier, production_block_id
    )

    verified_workers = []
    for row in verified_workers_db:
        (
            user_id,
            full_name,
            position,
            tasks_amount,
            verified_at,
            user_spool,
            verification,
        ) = row

        for worker in verified_workers:
            if worker.id == user_id:
                break
        else:
            # Create the VerifiedWorkerViewResponse object
            worker_response = VerifiedWorkerViewResponse(
                id=user_id,
                full_name=full_name,
                position=position,
                tasks_amount=tasks_amount,
                verified_at=verified_at,
                spools=[],
                verification=verification,
            )

            verified_workers.append(worker_response)

        spool_response = None
        for worker in verified_workers:
            if worker.id == user_id:
                if user_spool:
                    # Convert UserSpoolDB object to UserSpoolResponse
                    spool_response = UserSpoolResponse(
                        spool_id=user_spool.spool_id,
                        assigned_at=user_spool.assigned_at,
                    )
                    worker.spools.append(spool_response)
                break

    return verified_workers


@router.get("/workers/{worker_id}/tasks", response_model=List[TaskTasksView])
async def get_verified_workers_tasks_endpoint(
    worker_id: UUID,
    # brigadier: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
):
    brigadier_repository = BrigadierRepository(db_session)
    worker_tasks_db = await brigadier_repository.get_all_worker_tasks(worker_id)
    return worker_tasks_db


@router.get("/weekly_tasks", response_model=List[WeeklyTaskResponse])
async def get_weekly_tasks_endpoint(
    # production_block_id: UUID = None,
    _: UserDB = Depends(get_brigadier),
    db_session: Session = Depends(get_session),
    http_client=Depends(get_http_client),
):
    # brigadier_repository = BrigadierRepository(db_session)
    # verified_workers_db = await brigadier_repository.get_weekly_tasks(production_block_id)
    # return verified_workers_db

    ones_repository = OneSRepository(db_session, http_client)
    weekly_tasks = await ones_repository.get_weekly_tasks()
    return weekly_tasks
