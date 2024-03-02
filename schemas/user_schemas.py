import uuid
import datetime
from typing import Optional, Any, List
from uuid import UUID

from pydantic import BaseModel, Field, field_validator, model_validator, Json, RootModel, AliasChoices
from webauthn.helpers.cose import COSEAlgorithmIdentifier
from webauthn.helpers.structs import AuthenticatorTransport, CredentialDeviceType, AttestationConveyancePreference, UserVerificationRequirement, \
    AuthenticatorAttachment, ResidentKeyRequirement


class PasskeySchema(BaseModel):
    id: str
    public_key: str
    username: str
    sign_count: int
    is_discoverable_credential: bool | None
    device_type: CredentialDeviceType
    backed_up: bool
    transports: Optional[List[AuthenticatorTransport]]
    # TODO: Clear this at some point point in the future when we know we're setting it
    # aaguid: str = ""


class RegistrationOptionsSchema(BaseModel):
    username: str
    user_verification: UserVerificationRequirement
    attestation: AttestationConveyancePreference
    attachment: AuthenticatorAttachment
    algorithms: list[COSEAlgorithmIdentifier]
    discoverable_credential: ResidentKeyRequirement
