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


class TripDB(Base):
    __tablename__ = "trip"

    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str]
    place: Mapped[str]
    started_at: Mapped[datetime.datetime] = mapped_column(DateTime(timezone=True))
    finished_at: Mapped[datetime.datetime] = mapped_column(DateTime(timezone=True))


class TripMemberDB(Base):
    __tablename__ = "trip_member"

    trip_id: Mapped[int] = mapped_column(ForeignKey(f"{TripDB.__tablename__}.id"), primary_key=True)
    user_id: Mapped[UUID] = mapped_column(ForeignKey(f"user.id"), primary_key=True)
