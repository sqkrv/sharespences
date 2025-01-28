import uuid
import datetime
from base64 import b64decode
from typing import Optional, Any, List
from uuid import UUID

from pydantic import BaseModel, Field, field_validator, model_validator, Json, RootModel, AliasChoices, ConfigDict, alias_generators
from webauthn import base64url_to_bytes
from webauthn.helpers.cose import COSEAlgorithmIdentifier
from webauthn.helpers.structs import AuthenticatorTransport, CredentialDeviceType, AttestationConveyancePreference, UserVerificationRequirement, \
    AuthenticatorAttachment, ResidentKeyRequirement, RegistrationCredential, AuthenticatorAttestationResponse, AuthenticationCredential, \
    AuthenticatorAssertionResponse

from backend.schemas import CamelBaseModel


class PasskeySchema(BaseModel):
    id: str
    public_key: str
    username: str
    sign_count: int
    is_discoverable_credential: bool | None
    device_type: CredentialDeviceType
    backed_up: bool
    transports: Optional[List[AuthenticatorTransport]]


class _AuthenticatorAttestationResponse(CamelBaseModel, AuthenticatorAttestationResponse):
    client_data_json: bytes = Field(validation_alias='clientDataJSON')
    # attestation_object: bytes


class CustomRegistrationCredential(CamelBaseModel, RegistrationCredential):
    response: _AuthenticatorAttestationResponse
    user_id: str
    # username: str
    # display_name: str
    # email: str

    @field_validator('raw_id', mode='before')
    def convert_raw_id(cls, v: str) -> bytes:
        assert isinstance(v, str), 'raw_id is not a string'
        return base64url_to_bytes(v)

    @field_validator('response', mode='before')
    def convert_response(cls, data: dict) -> dict[..., bytes]:
        assert isinstance(data, dict), 'response is not a dictionary'
        return {k: base64url_to_bytes(v) for k, v in data.items()}


class RegistrationOptionsSchema(BaseModel):
    user_name: str
    display_name: str


class RegistrationSchema(BaseModel):
    user_id: str
    username: str
    display_name: str
    credentials: CustomRegistrationCredential


class TokenPayload(BaseModel):
    sub: UUID


class _AuthenticatorAssertionResponse(CamelBaseModel, AuthenticatorAssertionResponse):
    client_data_json: bytes = Field(validation_alias='clientDataJSON')
    # attestation_object: bytes


class MyCustomAuthenticationCredential(CamelBaseModel, AuthenticationCredential):
    response: _AuthenticatorAssertionResponse

    @field_validator('raw_id', mode='before')
    def convert_raw_id(cls, v: str) -> bytes:
        assert isinstance(v, str), 'raw_id is not a string'
        return b64decode(v)

    @field_validator('response', mode='before')
    def convert_response(cls, data: dict):
        assert isinstance(data, dict), 'response is not a dictionary'
        return {k: b64decode(v) for k, v in data.items()}
