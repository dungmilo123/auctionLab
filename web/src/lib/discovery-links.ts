export const lotURL = (slug: string) => `/lots/${encodeURIComponent(slug)}`
export const eventURL = (slug: string) =>
  `/auctions/${encodeURIComponent(slug)}`
