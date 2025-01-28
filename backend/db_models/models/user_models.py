import datetime
from typing import List

from sqlalchemy import Column, ForeignKey, Table, BigInteger, SmallInteger, DateTime, func, schema, ARRAY, text, Text
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.orm import relationship

from backend.db_models.db_base_class import Base
from uuid import UUID

# from backend.db_models.models import OperationDB


class UserDB(Base):
    __tablename__ = "user"

    id: Mapped[UUID] = mapped_column(server_default=text("gen_random_uuid()"), primary_key=True)
    username: Mapped[str] = mapped_column(unique=True)
    display_name: Mapped[str]
    email: Mapped[str] = mapped_column(unique=True)
    created_at: Mapped[datetime.datetime] = mapped_column(server_default=func.now())

    operations: Mapped["OperationDB"] = relationship(back_populates="user")
    passkeys: Mapped["PasskeyDB"] = relationship(back_populates="user")


class PasskeyDB(Base):
    __tablename__ = "passkey"

    id: Mapped[str] = mapped_column(primary_key=True, comment="Base64URL encoded CredentialID")
    user_id: Mapped[UUID] = mapped_column(ForeignKey(f"{UserDB.__tablename__}.id"))
    public_key: Mapped[str] = mapped_column(comment="Base64URL encoded PublicKey")
    name: Mapped[str]
    # transports: Mapped[List[str]] = mapped_column(ARRAY(Text))

    user: Mapped["UserDB"] = relationship()
