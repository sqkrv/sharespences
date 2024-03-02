import logging
import datetime

import httpx

from schemas.schemas import TinkoffSchemas

logger = logging.getLogger(__name__)


class TinkoffParser:
    def __init__(self, session_id: str):
        self.BANK_NAME = "Tinkoff"
        self.params = {
            'end': datetime.datetime.now().timestamp(),
            'start': datetime.datetime(2022, 1, 1, tzinfo=datetime.UTC).timestamp(),
            'sessionid': session_id
        }

    async def _get_raw_operations(self):
        with httpx.AsyncClient(params=self.params) as client:
            client: httpx.AsyncClient

            json_object = []

            response = await client.post(
                'https://www.tinkoff.ru/api/common/v1/operations'
            )

            if not response.is_success:
                logger.debug(f"Response was not OK:\n{response.text}")
                return

            operations = response.json()['payload']
            logger.debug(len(operations))

            json_object.extend(operations)

            return json_object

    async def convert_json_to_schema(self):
        operations_json = await self._get_raw_operations()
        for operation_json in operations_json:
            yield TinkoffSchemas.Operation(**operation_json)

    async def parse_operations(self):
        logger.info(f"Running {self.BANK_NAME} parser")

        operations = []
        async for operation in self.convert_json_to_schema():
            operation = TinkoffSchemas.Operation()
            operations.append(operation)

        logger.info(f"Finished parsing {self.BANK_NAME}")

        return operations
