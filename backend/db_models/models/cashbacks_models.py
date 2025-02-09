import datetime
from uuid import UUID

from sqlalchemy import ForeignKey
from sqlalchemy.orm import Mapped, mapped_column, relationship

from backend.db_models.db_base_class import Base


class CashbackDB(Base):
    """Cashback table representation object"""
    __tablename__ = "cashback"

    id: Mapped[int] = mapped_column(primary_key=True)
    user_id: Mapped[UUID] = mapped_column(ForeignKey("user.id"))
    category_id: Mapped[int] = mapped_column(ForeignKey("category.id"))
    start_date: Mapped[datetime.date]
    end_date: Mapped[datetime.date]
    percentage: Mapped[float]
    description: Mapped[str | None]
    super_cashback: Mapped[bool]

    category: Mapped["CategoryDB"] = relationship()
