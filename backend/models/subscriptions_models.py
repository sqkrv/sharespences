import datetime
from typing import TYPE_CHECKING
from uuid import UUID

from pydantic import computed_field
from sqlmodel import Field, DateTime, Interval, Relationship

from backend.models import Base, money_type

if TYPE_CHECKING:
    from .users_models import User
    from .common_models import BankCard


class Subscription(Base, table=True):
    id: int = Field(primary_key=True)
    name: str
    price: float = Field(sa_type=money_type)
    start_date: datetime.date
    recurrence_interval: datetime.timedelta = Field(sa_type=Interval)
    bank_card_id: int | None = Field(foreign_key="bank_card.id")
    is_active: bool = Field(default=True)
    notes: str | None
    icon_filename: str | None

    @computed_field()
    @property
    def next_payment_date(self) -> datetime.date | None:
        return self.start_date + self.recurrence_interval

    members: list["SubscriptionMember"] = Relationship(back_populates="subscription")
    bank_card: "BankCard" = Relationship(back_populates="subscriptions")

class SubscriptionMember(Base, table=True):
    subscription_id: int = Field(foreign_key="subscription.id", primary_key=True)
    user_id: UUID = Field(foreign_key="user.id", primary_key=True)
    since: datetime.datetime = Field(sa_type=DateTime(timezone=True))
    is_payer: bool = Field(default=False)

    subscription: Subscription = Relationship(back_populates="members")
    user: "User" = Relationship(back_populates="subscriptions")
