from sqlalchemy.ext.asyncio import create_async_engine
from sqlmodel import create_engine

from backend.core.config import settings

async_engine = create_async_engine(settings.postgres_async_url, pool_pre_ping=True)

sync_engine = create_engine(settings.postgres_url)
