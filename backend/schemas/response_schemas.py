import base64
import datetime
from typing import Optional, Any
from uuid import UUID

from pydantic import BaseModel, Field, field_serializer
from webauthn.helpers.structs import PublicKeyCredentialCreationOptions, PublicKeyCredentialUserEntity, PublicKeyCredentialRequestOptions, \
    PublicKeyCredentialDescriptor

from backend.schemas import CamelBaseModel
from backend.schemas.schema_enums.common_enums import Direction, Status


class GenericResponse(BaseModel):
    result: Optional[Any] = Field(None)
    error: Optional[Any] = Field(None)


class Transaction(BaseModel):
    id: UUID
    og_id: str
    timestamp: datetime.datetime
    title: str
    amount: float
    direction: Direction
    bank_card_id: int | None
    merchandiser_logo_url: str | None
    comment: str | None
    mcc_code: int
    # category_id: Mapped[int] = mapped_column(ForeignKey("category.id"), nullable=True)
    # loyalty_amount: Mapped[float] = mapped_column(nullable=True)
    status: Status | None
    # location: Geography


class MinimalOperation(BaseModel):
    id: UUID
    timestamp: datetime.datetime
    title: str
    amount: float
    direction: Direction
    status: Status | None


class _PublicKeyCredentialUserEntity(CamelBaseModel, PublicKeyCredentialUserEntity):
    ...

    @field_serializer('id')
    def serialize_challenge(self, v) -> str:
        return base64.b64encode(v).decode()


class RegistrationOptionsResponse(CamelBaseModel, PublicKeyCredentialCreationOptions):
    user: _PublicKeyCredentialUserEntity

    @field_serializer('challenge')
    def serialize_challenge(self, v) -> str:
        return base64.b64encode(v).decode()


class _PublicKeyCredentialDescriptor(CamelBaseModel, PublicKeyCredentialDescriptor):
    id: bytes

    @field_serializer('id')
    def serialize_id(self, v) -> str:
        return base64.urlsafe_b64encode(v).decode()


class AuthOptionsResponse(CamelBaseModel, PublicKeyCredentialRequestOptions):
    @field_serializer('challenge')
    def serialize_challenge(self, v) -> str:
        return base64.urlsafe_b64encode(v).decode()


class TokensResponse(BaseModel):
    access_token: str
    refresh_token: str
