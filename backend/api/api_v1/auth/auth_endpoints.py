import base64
import logging

import webauthn
from fastapi import APIRouter
from fastapi import Request, HTTPException, Response
from sqlalchemy import exc
from webauthn import generate_registration_options

from backend.api.api_v1.dependencies import DBSession
from backend.core import security
from backend.core.config import settings
from backend.db_models.models.user_models import UserDB, PasskeyDB
from backend.repositories.passkey_repository import PasskeysRepository
from backend.repositories.users_repository import UsersRepository
from backend.schemas.response_schemas import RegistrationOptionsResponse, AuthOptionsResponse, TokensResponse
from backend.schemas.user_schemas import CustomRegistrationCredential, MyCustomAuthenticationCredential, RegistrationOptionsSchema

router = APIRouter()
logger = logging.getLogger(__name__)


@router.post(
    "/register/options",
    response_model=RegistrationOptionsResponse,
    response_model_exclude_none=True
)
async def get_registration_options_endpoint(
        request: Request,
        registration_options: RegistrationOptionsSchema
):
    public_key = generate_registration_options(
        rp_id=settings.HOSTNAME,
        rp_name=settings.RP_ID,
        user_name=registration_options.user_name,
        user_display_name=registration_options.display_name,
    )
    request.session['webauthn_register_challenge'] = base64.urlsafe_b64encode(public_key.challenge).decode()
    return public_key


@router.post(
    "/register"
)
async def register_user_endpoint(
        registration_info: CustomRegistrationCredential,
        request: Request,
        db_session: DBSession
):
    users_repository = UsersRepository(db_session)
    passkeys_repository = PasskeysRepository(db_session)

    expected_challenge = base64.urlsafe_b64decode(request.session['webauthn_register_challenge'].encode())
    registration = webauthn.verify_registration_response(
        credential=registration_info,
        expected_challenge=expected_challenge,
        expected_rp_id=settings.RP_ID,
        expected_origin=settings.ORIGIN,
    )

    try:
        new_user_id = await users_repository.create_user(UserDB(
            username=registration_info.username,
            display_name=registration_info.display_name,
            email=registration_info.email,
        ))
    except exc.IntegrityError as e:
        logger.error(f"User creation failed: {e}")
        raise HTTPException(status_code=400, detail='User creation failed')

    await passkeys_repository.add_credential(PasskeyDB(
        id=base64.b64encode(registration.credential_id).decode(),
        user_id=new_user_id,
        name=request.headers.get('User-Agent'),
        public_key=base64.b64encode(registration.credential_public_key).decode(),
    ))
    print(base64.b64encode(registration.credential_id), base64.urlsafe_b64encode(registration.credential_id))


@router.get(
    "/auth/options",
    response_model=AuthOptionsResponse
)
async def auth_get(request: Request):
    public_key = webauthn.generate_authentication_options(
        rp_id=settings.RP_ID,
        allow_credentials=[],
    )
    request.session['webauthn_auth_challenge'] = base64.b64encode(public_key.challenge).decode()
    return public_key


@router.post(
    "/auth",
)
async def auth_post(
        credential: MyCustomAuthenticationCredential,
        request: Request,
        db_session: DBSession
):
    expected_challenge = base64.b64decode(request.session['webauthn_auth_challenge'].encode())
    passkeys_repository = PasskeysRepository(db_session)
    user_creds = await passkeys_repository.get_passkey(base64.b64encode(credential.raw_id).decode())

    if not user_creds:
        raise HTTPException(status_code=404, detail='user not found')

    try:
        webauthn.verify_authentication_response(
            credential=credential,
            expected_challenge=expected_challenge,
            expected_rp_id=settings.RP_ID,
            expected_origin=settings.ORIGIN,
            credential_public_key=base64.b64decode(user_creds.public_key),
            credential_current_sign_count=0
        )
    except Exception as e:
        logger.error(f"Authentication failed: {e}")
        raise HTTPException(status_code=401, detail='Authentication failed')

    access_token = security.create_access_token(user_creds.user_id)
    refresh_token = security.create_refresh_token(user_creds.user_id)
    return TokensResponse(access_token=access_token, refresh_token=refresh_token)
