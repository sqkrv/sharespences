import datetime
from typing import Optional, Literal, Any, List
from uuid import UUID

from geoalchemy2 import Geography
from pydantic import BaseModel, Field

from schemas.schema_enums.common_enums import Direction, Status


class AlfaBankSchemas:
    class Operation(BaseModel):
        id: str
        title: str | None
        click_reference: str = Field(validation_alias='clickReference')
        smart_vista_front_reference: Optional[int] = Field(validation_alias='smartVistaFrontReference')
        date_time: datetime.datetime = Field(validation_alias='dateTime')
        claim: "Claim" | None
        logo_url: str | None = Field(validation_alias='logoUrl')
        amount: "Amount"
        comment: str | None
        mcc: str
        category: "Category"
        direction: Literal["EXPENSE", "INCOME"]
        loyalty: "Loyalty"
        status: Literal["SUCCESS", "FAILURE"]
        actions: List["Action"]

    class Claim(BaseModel):
        type: str = Field(validation_alias='claimType')
        fee_type: str = Field(validation_alias='feeType')
        button_text: str = Field(validation_alias='buttonText')

    class Amount(BaseModel):
        value: float
        currency: str
        minor_units: float = Field(validation_alias='minorUnits')

    class Category(BaseModel):
        id: int
        name: str
        color: str
        icon: str
        is_client_category: bool = Field(validation_alias='isClientCategory')

    class Loyalty(BaseModel):
        title: str
        type: Literal["CASHBACK"]
        percent: Optional[Any]
        amount: "Amount"

    class Action(BaseModel):
        type: str
        label: str
        icon: "ActionIcon"

    class ActionIcon(BaseModel):
        name: str
        # asd:


class TinkoffSchemas:
    class Operation(BaseModel):
        loyalty_bonus_summary: "LoyaltyBonusSummary" = Field(validation_alias='loyaltyBonusSummary')
        is_offline: bool = Field(validation_alias='isOffline')
        icon: str
        is_inner: bool = Field(validation_alias='isInner')
        type: str
        subgroup: "Subgroup"
        is_dispute: bool = Field(validation_alias='isDispute')
        analytics_status: str = Field(validation_alias='analyticsStatus')
        authorization_id: str = Field(validation_alias='authorizationId')
        id: int
        status: str
        operation_transferred: bool = Field(validation_alias='operationTransferred')
        id_source_type: str = Field(validation_alias='idSourceType')
        loyalty_bonus: List["LoyaltyBonus"] = Field(validation_alias='loyaltyBonus')
        description: str
        debiting_time: "Time" = Field(validation_alias='debitingTime')
        is_templatable: bool = Field(validation_alias='isTemplatable')
        mcc: int
        category: "Category"
        ucid: int
        group: str
        mcc_string: str = Field(validation_alias='mccString')
        locations: List["Location"]
        cashback_amount: "Amount" = Field(validation_alias='cashbackAmount')
        cashback: float
        brand: "Brand"
        amount: "Amount"
        operation_time: "Time" = Field(validation_alias='operationTime')
        pos_id: str = Field(validation_alias='posId')
        spending_category: "SpendingCategory" = Field(validation_alias='spendingCategory')
        offers: List["Offer"]
        is_hce: bool = Field(validation_alias='isHce')
        additional_info: List["AdditionalInfo"] = Field(validation_alias='additionalInfo')
        compensation: str
        virtual_payment_type: int = Field(validation_alias='virtualPaymentType')
        account: str
        merchant: "Merchant"
        card: str
        loyalty_payment: List["LoyaltyPayment"] = Field(validation_alias='loyaltyPayment')
        tranche_creation_allowed: bool = Field(validation_alias='trancheCreationAllowed')
        card_present: bool = Field(validation_alias='cardPresent')
        account_amount: "Amount" = Field(validation_alias='accountAmount')
        is_external_card: bool = Field(validation_alias='isExternalCard')
        card_number: str = Field(validation_alias='cardNumber')

    class LoyaltyBonusSummary(BaseModel):
        amount: float

    class LoyaltyBonus(BaseModel):
        description: str
        icon: str
        loyaltyType: str
        amount: "LoyaltyBonusAmount"
        compensation_type: str = Field(validation_alias='compensationType')

    class LoyaltyBonusAmount(BaseModel):
        value: float
        loyalty_program_id: str = Field(validation_alias='loyaltyProgramId')
        loyalty: str
        name: str
        loyalty_steps: int = Field(validation_alias='loyaltySteps')
        loyalty_points_id: int = Field(validation_alias='loyaltyPointsId')
        loyalty_points_name: str = Field(validation_alias='loyaltyPointsName')
        loyalty_imagine: bool = Field(validation_alias='loyaltyImagine')
        partial_compensation: bool = Field(validation_alias='partialCompensation')

    class Subgroup(BaseModel):
        id: str
        name: str

    class Time(BaseModel):
        milliseconds: int

    class Category(BaseModel):
        id: int
        name: str

    class Location(BaseModel):
        latitude: float
        longitude: float

    class Amount(BaseModel):
        currency: "Currency"
        value: float

    class Currency(BaseModel):
        code: int
        name: str
        str_code: int = Field(validation_alias='strCode')

    class Brand(BaseModel):
        name: str
        link: str
        id: str
        rounded_logo: bool = Field(validation_alias='roundedLogo')

    # class Amount(BaseModel):
    #     currency: "Currency"
    #     value: float

    class SpendingCategory(BaseModel):
        name: str
        icon: str
        id: str
        base_color: str = Field(validation_alias='baseColor')

    class Offer(BaseModel):
        ...

    class AdditionalInfo(BaseModel):
        ...

    class Merchant(BaseModel):
        name: str
        region: "MerchantRegion"

    class MerchantRegion(BaseModel):
        country: str
        city: str

    class LoyaltyPayment(BaseModel):
        ...


class OperationSchema(BaseModel):
    # id: Mapped[UUID] = mapped_column(server_default=text("gen_random_uuid()"), primary_key=True)
    user_id: UUID
    og_id: str
    timestamp: datetime.datetime
    title: str
    amount: float
    direction: Direction
    # bank_id: Mapped[int | None] = mapped_column(ForeignKey(f"{BankDB.__tablename__}.id"))
    merchandiser_logo_url: str | None
    comment: str | None
    mcc_code: int | None
    category_id: int | None
    loyalty_amount: float | None
    status: Status
    location: Geography | None

    bank_card_id: int | None
    subscription_id: Mapped[int | None] = mapped_column(ForeignKey("subscription.id"))
    user_comment: Mapped[str | None]

    bank_card: Mapped["BankCardDB"] = relationship()
    category: Mapped["CategoryDB"] = relationship()
    subscription: Mapped["SubscriptionDB"] = relationship()
    user: Mapped["UserDB"] = relationship()
