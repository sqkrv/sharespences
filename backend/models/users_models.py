import datetime
from uuid import UUID

from pydantic import EmailStr
from sqlmodel import Field, DateTime, Relationship, Column, Uuid, BigInteger

from backend.models import Base
from backend.models.common_models import Transaction


class UserBase(Base):
    username: str | None = Field(unique=True)
    display_name: str | None

class UserPublic(UserBase):
    avatar_filename: str | None

class UserCreate(UserBase):  # todo stopped on implementing non-existing user (like when you add someone to the subscription and only then they register)
    email: EmailStr = Field(unique=True)

class User(UserBase, table=True):
    id: UUID = Field(None, sa_column=Column(Uuid, primary_key=True, server_default="gen_random_uuid()"))
    created_at: datetime.datetime | None = Field(None, sa_column=Column(DateTime(timezone=True), nullable=False, server_default="now()"))
    email: EmailStr = Field(unique=True)
    telegram_id: int | None = Field(None, sa_type=BigInteger, unique=True)

    operations: list["Transaction"] = Relationship(back_populates="user")
    passkeys: list["Passkey"] = Relationship(back_populates="user")


class Passkey(Base, table=True):
    id: str = Field(primary_key=True, sa_column_kwargs={'comment': "Base64URL encoded CredentialID"})
    user_id: UUID = Field(foreign_key="user.id")
    name: str
    public_key: str = Field(sa_column_kwargs={'comment': "Base64URL encoded PublicKey"})

    user: "User" = Relationship(back_populates="passkeys")
