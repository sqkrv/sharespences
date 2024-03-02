import datetime
from typing import Optional, Any, Union, Generic, TypeVar, List
from uuid import UUID

from geoalchemy2 import Geography
from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator, Json
from webauthn.helpers.structs import PublicKeyCredentialCreationOptions, PublicKeyCredentialUserEntity, PublicKeyCredentialParameters

from schemas.schema_enums.common_enums import Direction, Status


class GenericResponse(BaseModel):
    result: Optional[Any] = Field(None)
    error: Optional[Any] = Field(None)


class OperationResponseSchema:
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
    location: Geography


class _PublicKeyCredentialUserEntity(BaseModel, PublicKeyCredentialUserEntity):
    display_name: str = Field(validation_alias="displayName")


class RegistrationOptionsResponse(BaseModel, PublicKeyCredentialCreationOptions):
    user: _PublicKeyCredentialUserEntity
    pub_key_cred_params: List[PublicKeyCredentialParameters] = Field(validation_alias='pubKeyCredParams')

