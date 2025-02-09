import datetime
import uuid
from typing import List, Optional

from sqlalchemy import Column, ForeignKey, Table, BigInteger, SmallInteger, func, DateTime, Enum, CheckConstraint, text
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.orm import relationship
from geoalchemy2 import Geography, Geometry

from backend.db_models.db_base_class import Base
from uuid import UUID

from backend.db_models.models.user_models import UserDB
from backend.schemas.schema_enums.common_enums import Direction, Status, PaymentSystem


class BankDB(Base):
    __tablename__ = "bank"

    id: Mapped[int] = mapped_column(SmallInteger, primary_key=True)
    name: Mapped[str]
    logo_filename: Mapped[str | None]


class BankCardDB(Base):
    __tablename__ = "bank_card"

    id: Mapped[int] = mapped_column(primary_key=True)
    bank_id: Mapped[int] = mapped_column(ForeignKey("bank.id"))
    user_id: Mapped[int] = mapped_column(ForeignKey("user.id"))
    last_4_digits: Mapped[int] = mapped_column(CheckConstraint("LENGTH(last_4_digits::text) = 4"))
    payment_system: Mapped[PaymentSystem] = mapped_column(Enum(PaymentSystem, name="payment_system"))
    image_filename: Mapped[str | None]


class TransactionDB(Base):
    __tablename__ = "transaction"

    id: Mapped[UUID] = mapped_column(server_default=text("gen_random_uuid()"), primary_key=True)
    user_id: Mapped[UUID] = mapped_column(ForeignKey("user.id"))
    og_id: Mapped[str]
    timestamp: Mapped[datetime.datetime] = mapped_column(DateTime(timezone=True))
    title: Mapped[str]
    amount: Mapped[float]
    direction: Mapped[Direction] = mapped_column(Enum(Direction, name="direction"))
    bank_id: Mapped[int | None] = mapped_column(ForeignKey("bank.id"))
    merchandiser_logo_url: Mapped[str | None]
    bank_comment: Mapped[str | None]
    mcc_code: Mapped[int | None] = mapped_column(SmallInteger)
    category_id: Mapped[int | None] = mapped_column(ForeignKey("category.id"))
    loyalty_amount: Mapped[float | None]
    status: Mapped[Status] = mapped_column(Enum(Status, name="status"))
    location: Mapped[Geometry | None] = mapped_column(Geometry(geometry_type="POINT", srid=4326))

    bank_card_id: Mapped[int | None] = mapped_column(ForeignKey(f"{BankCardDB.__tablename__}.id"))
    # subscription_id: Mapped[int | None] = mapped_column(ForeignKey("subscription.id"))
    user_comment: Mapped[str | None]

    bank_card: Mapped["BankCardDB"] = relationship()
    category: Mapped["CategoryDB"] = relationship()
    # subscription: Mapped["SubscriptionDB"] = relationship()
    user: Mapped["UserDB"] = relationship()


class AttachmentDB(Base):
    __tablename__ = "attachment"

    id: Mapped[UUID] = mapped_column(server_default=text("gen_random_uuid()"), primary_key=True)
    filename: Mapped[str]
    media_type: Mapped[str | None]


class TransactionAttachmentDB(Base):
    __tablename__ = "transaction_attachment"

    transaction_id: Mapped[UUID] = mapped_column(ForeignKey(f"{TransactionDB.__tablename__}.id"), primary_key=True)
    attachment_id: Mapped[UUID] = mapped_column(ForeignKey(f"{AttachmentDB.__tablename__}.id"), primary_key=True)


class CategoryMCCDB(Base):
    __tablename__ = "category_mcc"

    category_id: Mapped[int] = mapped_column(ForeignKey("category.id"), primary_key=True)
    mcc_code: Mapped[int] = mapped_column(ForeignKey("mcc_code.code"), primary_key=True)


class MCCCodeDB(Base):
    """MCC code table representation object"""
    __tablename__ = "mcc_code"

    code: Mapped[int] = mapped_column(primary_key=True)
    name: Mapped[str]
    description: Mapped[str]


class CategoryDB(Base):
    """Cashback category table representation object"""
    __tablename__ = "category"

    id: Mapped[int] = mapped_column(primary_key=True)
    bank_id: Mapped[int] = mapped_column(ForeignKey("bank.id"))
    name: Mapped[str] = mapped_column()
    icon_filename: Mapped[str] = mapped_column(nullable=True)
    description: Mapped[str] = mapped_column(nullable=True)

    bank: Mapped["BankDB"] = relationship()
    mcc_codes: Mapped[List["MCCCodeDB"]] = relationship(secondary=CategoryMCCDB.__tablename__)


class PositionDB(Base):
    __tablename__ = "receipt_position"

    id: Mapped[UUID] = mapped_column(server_default=text("gen_random_uuid()"), primary_key=True)
    transaction_id: Mapped[UUID] = mapped_column(ForeignKey(f"{TransactionDB.__tablename__}.id"))
    name: Mapped[str]
    quantity: Mapped[float]
    amount: Mapped[int]


class TransactionUserDB(Base):
    __tablename__ = "transaction_user"

    transaction_id: Mapped[UUID] = mapped_column(ForeignKey(f"{TransactionDB.__tablename__}.id"), primary_key=True)
    user_id: Mapped[UUID] = mapped_column(ForeignKey(f"{UserDB.__tablename__}.id"), primary_key=True)
    position_id: Mapped[int | None] = mapped_column(ForeignKey(f"{PositionDB.__tablename__}.id"))
    equal_distribution: Mapped[bool ] = mapped_column()
