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


class SubscriptionDB(Base):
    __tablename__ = "subscription"

    id: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str]


class SubscriptionMemberDB(Base):
    __tablename__ = "subscription_member"

    subscription_id: Mapped[int] = mapped_column(ForeignKey("subscription.id"), primary_key=True)
    user_id: Mapped[int] = mapped_column(ForeignKey("user.id"), primary_key=True)
    since: Mapped[datetime.datetime] = mapped_column(DateTime(timezone=True), server_default=func.now())
