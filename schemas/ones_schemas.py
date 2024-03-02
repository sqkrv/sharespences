import uuid
import datetime
from typing import Optional, Any, List
from uuid import UUID

from pydantic import BaseModel, Field, field_validator, model_validator, Json, RootModel, AliasChoices


class OneSRole(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str


class OneSUser(BaseModel):
    id: UUID = Field(validation_alias='uid')
    full_name: str = Field(validation_alias='name')
    login: str
    password: str
    roles: List["OneSRole"]


class OneSProductionBlock(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str
    number: int
    # house_id: UUID = UUID("6d3d13bf-3cd0-4c3c-bc58-b33baa0f35a8")


class OneSARM(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str


class OneSClassifier(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str
    quality: None
    series: str | None = Field(None)
    amount: float | None = Field(None)
    cycle: Optional[int]
    is_amount_changeable: Optional[bool]
    category: "OneSARM"


class OneSSection(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str
    number: int


class OneSProductSort(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str


class OneSRow(BaseModel):
    id: UUID = Field(validation_alias='uid')
    name: str
    number: int
    section: "OneSSection"
    product: Optional["OneSProductSort"] = Field(None)
    # section_id: UUID = Field(validation_alias="section_uid")
    # section_name: str = Field(None)
    # section_number: int

    # @model_validator(mode='after')
    # def addressing_check(self):
    #     number = self.name.
    #     # if self.box_quality is not None or self.box_cell_id is not None:
    #     #     if not self.box_id:
    #     #         raise ValueError("box_id must be filled")
    #     return self


class OneSWeeklyTask(BaseModel):
    id: UUID = Field(validation_alias='uid')
    document_id: UUID = Field(validation_alias='uiddoc')
    production_block: Optional["OneSProductionBlock"]
    classifier: "OneSClassifier"
    amount: float
    measurement_unit: str
    quality: Optional[str]
    week_number: int

    already_done: float = 0.0                                           #


class OneSPallet(BaseModel):
    id: UUID = Field(validation_alias="uid")
    pallet_sheet_id: UUID | None

    @field_validator('id')
    @classmethod
    def id_is_not_nil_uuid(cls, v: UUID) -> UUID:
        if v.version is None:
            raise ValueError("Invalid pallet id")
        return v


class OneSZone(BaseModel):
    id: UUID = Field(validation_alias=AliasChoices('id', 'uid'))
    name: str


class OneSCell(BaseModel):
    id: UUID = Field(validation_alias=AliasChoices('id', 'uid'))
    name: str
    section: OneSSection | None = Field(None)


class OneSTask(BaseModel):
    id: UUID = Field(validation_alias='uid')
    document_id: UUID = Field(validation_alias='uiddoc', description="УИД документа")
    document_info: str = Field(validation_alias='infodoc', description="Информация о документе")
    production_block: Optional["OneSProductionBlock"]
    rows: List["OneSRow"]
    classifier: "OneSClassifier"
    worker_id: Optional[UUID] = Field(None)
    pallets: Optional[List["OneSPallet"]]
    move_from_zone: Optional["OneSZone"]
    move_from_cell: Optional["OneSCell"]
    move_to_zone: Optional["OneSZone"]
    move_to_cell: Optional["OneSCell"]
    warehouse_zone: Optional["OneSZone"]


class OneSFinishedBox(BaseModel):
    id: UUID
    cell_id: UUID | None
    quality: str | None = Field(None)


class OneSFinishedPallet(BaseModel):
    id: UUID = Field(serialization_alias='id')
    boxes: List["OneSFinishedBox"]
    quality: Optional[str]
    quality_control_form: Any | None
    zone_cell_name: str | None
    pallet_sheet_id: UUID | None


class OneSFinishedTask(BaseModel):
    id: UUID = Field(serialization_alias='id')
    document_id: UUID = Field(serialization_alias='iddoc', description="УИД документа недельного плана")
    external_number: int
    classifier_id: UUID
    worker_id: UUID
    finished: bool
    rating: Optional[int]
    amount: Optional[int]
    time_spent: Optional[int]
    pallets: List["OneSFinishedPallet"]
    boxes: List["OneSFinishedBox"]
    rows: List[UUID]
    scales_id: Optional[UUID]
    packing_remainder: Optional[int]
    weight: float | None
    # pallet_sheet_id: UUID | None = Field(None)


class _OneSScalesClassifierCategory(BaseModel):
    id: UUID = Field(serialization_alias='uid')


class _OneSScalesClassifier(BaseModel):
    category: "_OneSScalesClassifierCategory"


class OneSScales(BaseModel):
    id: UUID = Field(serialization_alias='uid')
    document_id: UUID = Field(serialization_alias='iddoc', description="УИД документа недельного плана")
    scales_id: UUID = Field(serialization_alias='uidscale')
    classifier: "_OneSScalesClassifier"


class OneSScalesResponse(BaseModel):
    pallet_sheet_id: UUID | None = Field(None, validation_alias='QRCodDoc')
    pallet_sheet_base64: str | None = Field(None)
    weight: Optional[float]


class OneSKPProduct(BaseModel):
    name: str
    article: str = Field(serialization_alias='articul')
    unit: str
    price: float
    barcodes: List[str] = Field(serialization_alias='barcode')


class OneSKP(BaseModel):
    date: datetime.datetime
    number: Optional[str]
    name: str
    INN: str
    KPP: str
    products: List["OneSKPProduct"]
