import datetime
from uuid import UUID

from pydantic import EmailStr
from sqlmodel import Field, DateTime, Relationship, Column, Uuid

from backend.models import Base
from backend.models.common_models import Transaction


class UserBase(Base):
    username: str = Field(unique=True)
    display_name: str
    email: EmailStr = Field(unique=True)

class User(UserBase, table=True):
    id: UUID | None = Field(None, sa_column=Column(Uuid, primary_key=True, server_default="gen_random_uuid()"))
    created_at: datetime.datetime | None = Field(None, sa_column=Column(DateTime(timezone=True), nullable=False, server_default="now()"))

    operations: list["Transaction"] = Relationship(back_populates="user")
    passkeys: list["Passkey"] = Relationship(back_populates="user")


class UserCreate(UserBase):
    ...


class Passkey(Base, table=True):
    id: str = Field(primary_key=True, sa_column_kwargs={'comment': "Base64URL encoded CredentialID"})
    user_id: UUID = Field(foreign_key="user.id")
    name: str
    public_key: str = Field(sa_column_kwargs={'comment': "Base64URL encoded PublicKey"})

    user: "User" = Relationship(back_populates="passkeys")
