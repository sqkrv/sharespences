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


class CashbackDB(Base):
    """Cashback for a month table representation object"""
    __tablename__ = "cashback"

    id: Mapped[int] = mapped_column(primary_key=True)
    date: Mapped[datetime.date] = mapped_column(DateTime(timezone=True))
    bank_id: Mapped[int] = mapped_column(ForeignKey("bank.id"))
    # category_id = ForeignKeyField(Category, backref='cashbacks')
    # beneficiary_id = ForeignKeyField(Beneficiary, null=False, backref='cashbacks')
    # percent = SmallIntegerField()
    # start_date = DateField(null=False)
    # end_date = DateField(null=False)
    # overwritten_description = TextField(null=False)
    # super = BooleanField(default=False)
