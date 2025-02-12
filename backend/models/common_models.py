import datetime
from typing import TYPE_CHECKING
from uuid import UUID

# from geoalchemy2 import Geometry
from sqlalchemy import SmallInteger, DateTime, Enum, CheckConstraint
from sqlmodel import Field, Relationship

from backend.db import Base
from backend.models.enums.common_enums import Direction, Status, PaymentSystem

if TYPE_CHECKING:
    from .subscriptions_models import Subscription
    from .user_models import User



class Bank(Base, table=True):
    id: int = Field(sa_type=SmallInteger, primary_key=True)
    name: str
    logo_filename: str | None


class BankCard(Base, table=True):
    id: int = Field(primary_key=True)
    bank_id: int = Field(foreign_key="bank.id")
    user_id: int = Field(foreign_key="user.id")
    last_4_digits: int = CheckConstraint("LENGTH(last_4_digits::text) = 4")
    payment_system: PaymentSystem = Field(sa_type=Enum(PaymentSystem, name="payment_system"))
    image_filename: str | None


class Transaction(Base, table=True):
    id: UUID = Field(primary_key=True)
    user_id: UUID = Field(foreign_key="user.id")
    og_id: str
    timestamp: datetime.datetime = Field(sa_type=DateTime(timezone=True))
    title: str
    amount: float
    direction: Direction = Field(sa_type=Enum(Direction, name="direction"))
    bank_id: int | None = Field(foreign_key="bank.id")
    merchandiser_logo_url: str | None
    bank_comment: str | None
    mcc_code: int | None = Field(sa_type=SmallInteger)
    category_id: int | None = Field(foreign_key="category.id")
    loyalty_amount: float | None
    status: Status = Field(sa_type=Enum(Status, name="status"))
    # location: Geometry | None = Field(sa_type=Geometry(geometry_type="POINT", srid=4326))

    bank_card_id: int | None = Field(foreign_key=f"{BankCard.__tablename__}.id")
    subscription_id: int | None = Field(foreign_key="subscription.id")
    user_comment: str | None

    bank_card: "BankCard" = Relationship()
    category: "Category" = Relationship()
    # subscription: "Subscription" = Relationship()
    user: "User" = Relationship()


class Attachment(Base, table=True):
    id: UUID = Field(primary_key=True)
    filename: str
    media_type: str | None


class TransactionAttachment(Base, table=True):
    transaction_id: UUID = Field(foreign_key=f"{Transaction.__tablename__}.id", primary_key=True)
    attachment_id: UUID = Field(foreign_key=f"{Attachment.__tablename__}.id", primary_key=True)


class CategoryMCC(Base, table=True):
    category_id: int = Field(foreign_key="category.id", primary_key=True)
    mcc_code: int = Field(foreign_key="mcc_code.code", primary_key=True)


class MCCCode(Base, table=True):
    """MCC code table representation object"""

    code: int = Field(primary_key=True)
    name: str
    description: str


class CategoryBase(Base):
    """Cashback category table representation object"""

    id: int = Field(primary_key=True)
    bank_id: int = Field(foreign_key="bank.id")
    name: str
    icon_filename: str | None
    description: str | None


class Category(CategoryBase, table=True):
    bank: "Bank" = Relationship()
    mcc_codes: list["MCCCode"] = Relationship(sa_relationship_kwargs={'secondary': f"{CategoryMCC.__tablename__}"})


class CategoryWithMCCCodes(CategoryBase):
    """Cashback category table representation object"""
    mcc_codes: list[int]


class ReceiptPosition(Base, table=True):
    id: UUID = Field(primary_key=True)
    transaction_id: UUID = Field(foreign_key=f"{Transaction.__tablename__}.id")
    name: str
    quantity: float
    amount: int


class TransactionUser(Base, table=True):
    transaction_id: UUID = Field(foreign_key=f"{Transaction.__tablename__}.id", primary_key=True)
    user_id: UUID = Field(foreign_key="user.id", primary_key=True)
    position_id: int | None = Field(foreign_key=f"{ReceiptPosition.__tablename__}.id")
    equal_distribution: bool
