from logging.config import dictConfig as loggerDictConfig
import logging

from fastapi import FastAPI
from fastapi.openapi.utils import get_openapi
from fastapi.responses import ORJSONResponse
from starlette.middleware.cors import CORSMiddleware
from starlette.middleware.sessions import SessionMiddleware
from starlette.staticfiles import StaticFiles

from backend.core.config import settings, logging_conf
from backend.api.api_v1.api import api_router

loggerDictConfig(logging_conf)

openapi_url = f"{settings.API_V1_PATH}/openapi.json"

app = FastAPI(title=settings.project_name,
              openapi_url=openapi_url,
              debug=settings.debug,
              docs_url='/api/docs',
              redoc_url='/api/redoc',
              version='0.0.1',
              default_response_class=ORJSONResponse)

app.include_router(api_router, prefix=settings.API_V1_PATH)
app.mount("", StaticFiles(directory="test_auth"), name="static")

# app.state.transactions_waiting = 0

# @app.on_event('startup')
# async def init_runners():
#     app.state.runners = await start_qiwi_runners()
#
#
# @app.on_event('shutdown')
# async def stop_runners_event():
#     await stop_runners(app.state.runners)


# Set all CORS enabled origins
if settings.BACKEND_CORS_ORIGINS:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.BACKEND_CORS_ORIGINS,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

app.add_middleware(SessionMiddleware, secret_key=settings.secret_key)


if settings.debug:
    logging.getLogger('sqlalchemy.engine').setLevel(logging.INFO)


# def custom_openapi():
#     if app.openapi_schema:
#         return app.openapi_schema
#     openapi_schema = get_openapi(
#         title=settings.project_name,
#         version=app.version,
#         description="Sharespences API",
#         routes=app.routes,
#     )
#     # openapi_schema['components']['securitySchemes'] = {
#     #     'cookieAuth': {
#     #         'type': 'apiKey',
#     #         'in': 'cookie',
#     #         'name': 'TOKEN',
#     #         'description': 'Enter JWT token, where JWT is the access token'
#     #     }
#     # }
#     app.openapi_schema = openapi_schema
#     return app.openapi_schema
#
#
# app.openapi = custom_openapi

# if settings.ONES_INTEGRATION:
#     start_runners()
#     logging.info("Runners started")
