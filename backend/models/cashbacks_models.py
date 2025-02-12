import datetime
from uuid import UUID
from typing import TYPE_CHECKING

from sqlmodel import Field, Relationship

from backend.db.db_base_class import Base

if TYPE_CHECKING:
    from .common_models import Category


class CashbackBase(Base):
    """Cashback table representation object"""
    category_id: int = Field(foreign_key="category.id")
    start_date: datetime.date
    end_date: datetime.date
    percentage: float
    super_cashback: bool


class Cashback(CashbackBase, table=True):
    id: int = Field(primary_key=True)
    user_id: UUID = Field(foreign_key="user.id")

    category: "Category" = Relationship()


class CashbackAdd(CashbackBase):
    ...
