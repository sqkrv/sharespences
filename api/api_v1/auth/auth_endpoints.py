import datetime
import json
from pathlib import Path
from typing import List, Annotated

from fastapi import APIRouter, Depends, Query, status, UploadFile, Form, File
import logging
from sqlalchemy.orm import Session
from starlette.responses import JSONResponse
from webauthn import generate_registration_options, options_to_json, verify_registration_response

from api.api_v1.dependencies import get_session
from core.config import settings
from repositories.passkey_repository import PasskeyRepository
from schemas.response_schemas import RegistrationOptionsResponse
from schemas.user_schemas import RegistrationOptionsSchema

# from repositories.operations_repository import
# from schemas.response_schemas import OperationResponseSchema

# from schemas.schema_enums.news_enums import ArticleType


router = APIRouter()
logger = logging.getLogger(__name__)

passkey_repository = PasskeyRepository()


@router.post(
    "/registration/options",
    response_model=RegistrationOptionsResponse
)
async def registration_options_endpoint(
        options: RegistrationOptionsSchema
):
    """
    Generate options for a WebAuthn registration ceremony
    """
    registration_options = generate_registration_options(
        rp_id=settings.RP_ID,
        rp_name=settings.RP_NAME,
        user_name=options.username,
        attestation=options.attestation,
        supported_pub_key_algs=options.algorithms
    )

    options_json = json.loads(options_to_json(registration_options))

    # # Tack on hints (till py_webauthn learns about hints and we can handle it in the service)
    # options_json["hints"] = options_hints
    #
    # # Add in credProps extension
    # options_json["extensions"] = {
    #     "credProps": True,
    # }

    return options_json


@router.post(
    "/registration/verification",
    response_model=RegistrationOptionsResponse
)
async def registration_verification_endpoint(
        registration
):
    """
    Verify the response from a WebAuthn registration ceremony
    """

    registration_verification = verify_registration_response(

    )

    body_json: dict = json.loads(request.body)

    response_form = RegistrationResponseForm(body_json)

    if not response_form.is_valid():
        return JsonResponseBadRequest(dict(response_form.errors.items()))

    form_data = response_form.cleaned_data
    username: str = form_data["username"]
    webauthn_response: dict = form_data["response"]

    registration_service = RegistrationService()

    try:
        (verification, options) = registration_service.verify_registration_response(
            username=username,
            response=webauthn_response,
        )

        _response: dict = webauthn_response.get("response", {})
        transports: list = _response.get("transports", [])

        # Try to determine if the credential we got is a discoverable credential
        is_discoverable_credential = None

        # If credProps.rk is defined then use that as the most definitive signal
        extensions: dict = webauthn_response.get("clientExtensionResults", {})
        ext_cred_props: dict | None = extensions.get("credProps", None)
        if ext_cred_props is not None:
            ext_cred_props_rk: bool | None = ext_cred_props.get("rk", None)
            if ext_cred_props_rk is not None:
                is_discoverable_credential = bool(ext_cred_props_rk)

        # If we can't determine this using credProps then let's look at the registration options
        if is_discoverable_credential is None:
            if options.authenticator_selection.resident_key == ResidentKeyRequirement.REQUIRED:
                is_discoverable_credential = True

        # Store credential for later
        credential_service = CredentialService()
        credential_service.store_credential(
            username=username,
            verification=verification,
            transports=transports,
            is_discoverable_credential=is_discoverable_credential,
        )
    except Exception as err:
        return JsonResponseBadRequest({
                                          "error": str(err)})

    return JsonResponse({
                            "verified": True})