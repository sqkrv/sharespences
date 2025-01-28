from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy import create_engine
from sqlalchemy.orm import sessionmaker, Session

from backend.core.config import settings

async_engine = create_async_engine(settings.postgres_async_url, pool_pre_ping=True)
AsyncSessionLocal = sessionmaker(bind=async_engine, expire_on_commit=False, class_=AsyncSession)

sync_engine = create_engine(settings.postgres_url)
SessionLocal = sessionmaker(bind=sync_engine, expire_on_commit=False, class_=Session)
