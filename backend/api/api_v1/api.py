from fastapi import APIRouter

from backend.api.api_v1.auth import auth_endpoints
from backend.api.api_v1.cashbacks import cashbacks_endpoints

api_router = APIRouter()

api_router.include_router(auth_endpoints.router, prefix="/auth", tags=["Authentication"])
api_router.include_router(cashbacks_endpoints.router, prefix="/cashbacks", tags=["Cashbacks"])
