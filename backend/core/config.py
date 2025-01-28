import os
from pathlib import Path
from typing import List

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    environment: str | None = None
    debug: bool = os.environ.get("DEBUG", False)

    # DATABASE_BASE_URL: str = f"postgres:{os.environ.get("POSTGRES_PASSWORD")}@{os.environ.get("POSTGRES_HOST")}:{os.environ.get("POSTGRES_PORT")}/postgres"
    DATABASE_BASE_URL: str = os.environ.get("DATABASE_URL_PART", None)
    HOSTNAME: str = os.environ.get("HOSTNAME", None)
    ORIGIN: str = f"http://{HOSTNAME}:8777"
    RP_ID: str = HOSTNAME
    RP_NAME: str = "Sharespences Co."

    postgres_async_url: str = "postgresql+asyncpg://" + DATABASE_BASE_URL
    postgres_url: str = "postgresql://" + DATABASE_BASE_URL
    SQLALCHEMY_DATABASE_URI: str = postgres_async_url
    DOWNLOADS_PATH: Path = Path("downloads")
    MEDIA_PATH: Path = Path("media")

    project_name: str = "Sharespences"
    API_V1_PATH: str = "/api/v1"

    ACCESS_TOKEN_EXPIRE_MINUTES: int = 60 * 3
    REFRESH_TOKEN_EXPIRE_MINUTES: int = 60 * 24 * 30  # 60 minutes * 24 hours * 90 days = 30 days
    ACCESS_TOKEN_SECRET_KEY: str | None = os.environ.get("JWT_ACCESS_TOKEN_SECRET", None)
    REFRESH_TOKEN_SECRET_KEY: str | None = os.environ.get("JWT_REFRESH_TOKEN_SECRET", None)
    BACKEND_CORS_ORIGINS: List[str] = ["localhost", "*"]

    secret_key: str = 'some-random-string-for-now'


settings = Settings()


logging_conf = {
    'version': 1,
    'disable_existing_loggers': False,
    'formatters': {
        'console': {
            'format': '{name} {levelname} {asctime} {module} {process:d} {thread:d} {message}',
            'style': '{',
        },
    },
    'handlers': {
        'console': {
            'class': 'logging.StreamHandler',
            'formatter': 'console'
        },
    },
    'loggers': {
        '': {
            'level': os.getenv('LOG_LEVEL', 'DEBUG'),
            'handlers': ['console', ],
            'propagate': True
        }
    }
}
