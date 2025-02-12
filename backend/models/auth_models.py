import base64
from base64 import b64decode
from uuid import UUID

from pydantic import Field, field_validator, EmailStr
from pydantic import field_serializer
from webauthn import base64url_to_bytes
from webauthn.helpers.structs import AuthenticatorTransport, CredentialDeviceType, RegistrationCredential, AuthenticatorAttestationResponse, \
    AuthenticationCredential, AuthenticatorAssertionResponse
from webauthn.helpers.structs import PublicKeyCredentialCreationOptions, PublicKeyCredentialUserEntity, PublicKeyCredentialRequestOptions, \
    PublicKeyCredentialDescriptor

from backend.db import Base
from backend.models import CamelBase


class _PublicKeyCredentialUserEntity(CamelBase, PublicKeyCredentialUserEntity):
    @field_serializer('id')
    def serialize_challenge(self, v) -> str:
        return base64.b64encode(v).decode()


class RegistrationOptionsResponse(CamelBase, PublicKeyCredentialCreationOptions):
    user: _PublicKeyCredentialUserEntity

    @field_serializer('challenge')
    def serialize_challenge(self, v) -> str:
        return base64.b64encode(v).decode()


class _PublicKeyCredentialDescriptor(CamelBase, PublicKeyCredentialDescriptor):
    id: bytes

    @field_serializer('id')
    def serialize_id(self, v) -> str:
        return base64.urlsafe_b64encode(v).decode()


class AuthOptionsResponse(CamelBase, PublicKeyCredentialRequestOptions):
    @field_serializer('challenge')
    def serialize_challenge(self, v) -> str:
        return base64.urlsafe_b64encode(v).decode()


class _AuthenticatorAttestationResponse(CamelBase, AuthenticatorAttestationResponse):
    client_data_json: bytes = Field(validation_alias='clientDataJSON')
    # attestation_object: bytes


class CustomRegistrationCredential(CamelBase, RegistrationCredential):
    response: _AuthenticatorAttestationResponse
    user_id: str
    username: str
    display_name: str
    email: EmailStr

    @field_validator('raw_id', mode='before')
    def convert_raw_id(cls, v: str) -> bytes:
        assert isinstance(v, str), 'raw_id is not a string'
        return base64url_to_bytes(v)

    @field_validator('response', mode='before')
    def convert_response(cls, data: dict) -> dict[..., bytes]:
        assert isinstance(data, dict), 'response is not a dictionary'
        return {k: base64url_to_bytes(v) for k, v in data.items()}


class _AuthenticatorAssertionResponse(CamelBase, AuthenticatorAssertionResponse):
    client_data_json: bytes = Field(validation_alias='clientDataJSON')
    # attestation_object: bytes


class MyCustomAuthenticationCredential(CamelBase, AuthenticationCredential):
    response: _AuthenticatorAssertionResponse

    @field_validator('raw_id', mode='before')
    def convert_raw_id(cls, v: str) -> bytes:
        assert isinstance(v, str), 'raw_id is not a string'
        return b64decode(v)

    @field_validator('response', mode='before')
    def convert_response(cls, data: dict):
        assert isinstance(data, dict), 'response is not a dictionary'
        return {k: b64decode(v) for k, v in data.items()}


class TokensResponse(Base):
    access_token: str
    refresh_token: str


class PasskeySchema(Base):
    id: str
    public_key: str
    username: str
    sign_count: int
    is_discoverable_credential: bool | None
    device_type: CredentialDeviceType
    backed_up: bool
    transports: list[AuthenticatorTransport] | None


class RegistrationOptionsSchema(Base):
    user_name: str
    display_name: str


class RegistrationSchema(Base):
    user_id: str
    username: str
    display_name: str
    credentials: CustomRegistrationCredential


class TokenPayload(Base):
    sub: UUID
