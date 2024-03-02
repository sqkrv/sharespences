import datetime
import uuid
from typing import List

from sqlalchemy import Column, ForeignKey, Table, BigInteger, SmallInteger, func, DateTime, Enum
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.orm import relationship
from geoalchemy2 import Geography

from db_models.db_base_class import Base
from uuid import UUID

from db_models.models import UserDB
from schemas.schema_enums.common_enums import Direction, Status


class ArticleDB(Base):
    __tablename__ = "news"

    id: Mapped[int] = mapped_column(primary_key=True)
    title: Mapped[str]
    text: Mapped[str]
