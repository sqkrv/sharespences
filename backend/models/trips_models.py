import datetime

from sqlalchemy import ForeignKey, DateTime
from sqlalchemy.orm import Mapped, mapped_column

from backend.models import Base
from uuid import UUID


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
