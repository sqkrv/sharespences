from fastapi import APIRouter

# from backend.api.api_v1.operations import operations_endpoints
from backend.api.api_v1.auth import auth_endpoints

api_router = APIRouter()

api_router.include_router(auth_endpoints.router, prefix="/auth", tags=["Authentication"])
# api_router.include_router(operations_endpoints.router, prefix="/operations", tags=["Operations"])
