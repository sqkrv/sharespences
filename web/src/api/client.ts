import createClient from "openapi-fetch";
import type { paths, components } from "./schema";

export type Schemas = components["schemas"];
export type Bank = Schemas["BankDTO"];
export type BankClient = Schemas["BankClientDTO"];
export type Card = Schemas["CardDTO"];
export type Program = Schemas["ProgramDTO"];
export type Tier = Schemas["TierDTO"];
export type CanonicalCategory = Schemas["CanonicalCategoryDTO"];
export type BankCategory = Schemas["BankCategoryDTO"];
export type CategoryOffer = Schemas["CategoryOfferDTO"];
export type HelperRow = Schemas["HelperRowDTO"];
export type LookupEntry = Schemas["LookupEntryDTO"];
export type PartnerOffer = Schemas["PartnerOfferDTO"];
export type OfferPeriodItem = Schemas["OfferPeriodListItem"];
export type RecognitionJob = Schemas["RecognitionJobDTO"];
export type RecognitionDraft = Schemas["RecognitionDraftDTO"];
export type Friend = Schemas["FriendDTO"];
export type FoundUser = Schemas["FoundUserDTO"];
export type FriendRequest = Schemas["RequestDTO"];
export type FriendInvite = Schemas["InviteDTO"];
export type FriendSharing = Schemas["SharingDTO"];
export type FriendCashback = Schemas["FriendCashbackDTO"];
export type FriendSharedClient = Schemas["FriendSharedClientDTO"];
export type FriendOffer = Schemas["FriendOfferDTO"];
export type FriendPeriod = Schemas["FriendPeriodDTO"];
export type PerkClient = Schemas["PerkClientDTO"];
export type Perk = Schemas["PerkDTO"];
export type PerkQuota = Schemas["PerkQuotaDTO"];
export type PerkHistoryQuota = Schemas["PerkHistoryQuotaDTO"];
export type PerkEvent = Schemas["PerkEventDTO"];
export type PerkSnapshot = Schemas["PerkSnapshotDTO"];

// huma error body (ErrorModel)
type ErrorBody = { title?: string; detail?: string; errors?: { message?: string; location?: string }[] };

export class ApiError extends Error {
  status: number;
  body?: ErrorBody;
  constructor(status: number, body?: unknown) {
    const b = (body ?? undefined) as ErrorBody | undefined;
    super(b?.detail || b?.errors?.[0]?.message || b?.title || `Ошибка ${status}`);
    this.status = status;
    this.body = b;
  }
}

export const api = createClient<paths>();

// unwrap turns openapi-fetch's {data, error, response} into data-or-throw,
// so TanStack Query sees a plain promise.
export function unwrap<D>(r: { data?: D; error?: unknown; response: Response }): D {
  if (!r.response.ok) throw new ApiError(r.response.status, r.error);
  return r.data as D;
}

// Attachment upload is multipart (outside the JSON client on purpose).
export async function uploadAttachment(file: File): Promise<{ id: string; filename: string }> {
  const fd = new FormData();
  fd.append("file", file);
  const resp = await fetch("/api/v1/attachments", { method: "POST", body: fd });
  if (!resp.ok) throw new ApiError(resp.status, await resp.json().catch(() => undefined));
  return resp.json();
}

export function attachmentURL(id: string): string {
  return `/api/v1/attachments/${id}/content`;
}
