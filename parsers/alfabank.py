import logging

import httpx

from parsers import Parser

logger = logging.getLogger(__name__)


class AlfabankParser(Parser):
    def __init__(self,
                 cookie_header: str,
                 xsrf_token_header: str):
        self.BANK_NAME = "Alfabank"
        self.headers = {
            'Cookie': cookie_header,
            'X-XSRF-TOKEN': xsrf_token_header,
        }

        self.total_operations_to_save = 1500
        self.block_size = 100

    async def parse_operations(self):
        json_object = []

        with httpx.AsyncClient(headers=self.headers) as client:
            client: httpx.AsyncClient
            logger.info(f"Running {self.BANK_NAME} parser")

            for block_number in range(1, self.total_operations_to_save // self.block_size):
                logger.debug(f"{block_number}/{self.total_operations_to_save // self.block_size}")

                json_data = {
                    'size': self.block_size,
                    'page': block_number,
                }

                response = await client.post(
                    'https://web.alfabank.ru/api/operations-history-api/operations',
                    json=json_data
                )

                if not response.is_success:
                    logger.debug(f"Response was not OK:\n{response.text}")
                    break

                operations = response.json()['operations']
                logger.debug(len(operations))

                json_object.extend(operations)

            logger.info(f"Finished parsing {self.BANK_NAME}")

            return json_object
            # with open("../operations2.json", 'w') as f:
            #     json.dump(json_object, f)
