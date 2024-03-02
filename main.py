import asyncio
from logging.config import dictConfig
import logging

from fastapi import FastAPI
from fastapi.openapi.utils import get_openapi
from starlette.middleware.cors import CORSMiddleware
# from starlette.middleware import

from core.config import settings, logging_conf
from api.api_v1.api import api_router

dictConfig(logging_conf)

openapi_url = f"{settings.api_v1_path}/openapi.json"

app = FastAPI(title=settings.project_name,
              openapi_url=openapi_url,
              debug=settings.debug,
              docs_url='/api/docs',
              redoc_url='/api/redoc',
              version='0.0.1')

# app.mount("/static", StaticFiles(directory="static"), name="static")
app.include_router(api_router, prefix=settings.api_v1_path)

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
if settings.backend_cors_origins:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.backend_cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )


if settings.debug:
    logging.getLogger('sqlalchemy.engine').setLevel(logging.INFO)


def custom_openapi():
    if app.openapi_schema:
        return app.openapi_schema
    openapi_schema = get_openapi(
        title=settings.project_name,
        version=app.version,
        description="Sharespences API",
        routes=app.routes,
    )
    openapi_schema['components']['securitySchemes'] = {
        'cookieAuth': {
            'type': 'apiKey',
            'in': 'cookie',
            'name': 'TOKEN',
            'description': 'Enter JWT token, where JWT is the access token'
        }
    }
    app.openapi_schema = openapi_schema
    return app.openapi_schema


app.openapi = custom_openapi

# if settings.ONES_INTEGRATION:
#     start_runners()
#     logging.info("Runners started")
