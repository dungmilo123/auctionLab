// Exact legacy stock-field presentation only. Never rewrite API/storage values.
import type { PublicEvent, PublicLot, Fulfillment, Image } from '@/api/catalog'
import type { Summary } from '@/api/fulfillment'
import type { Order } from '@/api/order'
const titles = ["Folding travel camera", "Silver rangefinder study", "Twin lens portrait camera", "Pocket point and shoot", "Leather travel album", "Ceramic picture frame", "Field watch with canvas strap", "Open face pocket watch", "Walnut desk clock", "Brass bud vase", "Oak mantel clock", "Square travel watch", "Midnight jazz record", "Chamber music record", "Sunday soul record", "Silver stereo receiver", "Belt drive turntable", "Walnut bookshelf speakers"] as const
const equivalent = (value: string, original: string, replacement: string) => value === original ? replacement : value
function identity(path: string) {
  const match = /^\/site-assets\/catalog\/lot-(\d{2})(?:-detail)?\.svg$/.exec(path)
  const n = match ? Number(match[1]) : 0
  return n >= 1 && n <= titles.length ? titles[n - 1] : undefined
}
function imageDisplay(image: Image, title: string): Image {
  return { ...image, alt: equivalent(equivalent(image.alt, title + ' demo illustration', title), title + ' illustrated detail', title + ', detail') }
}
function fulfillmentDisplay(f: Fulfillment): Fulfillment {
  return { ...f,
    shipping_instructions: equivalent(f.shipping_instructions, 'Demo delivery: protective packaging; fee disclosed in USD.', 'Protective packaging; delivery fee disclosed in USD.'),
    pickup_instructions: equivalent(f.pickup_instructions, 'Demo pickup by arrangement; no real collection address.', 'Pickup by arrangement; collection address is not provided.') }
}
export function displayLot<T extends PublicLot>(lot: T): T {
  const title = lot.demo ? identity(lot.images[0]?.path ?? '') : undefined
  if (!title) return lot
  return { ...lot,
    description: equivalent(lot.description, 'Illustrative vintage object for catalog preparation. This is not a real sale listing.', 'A vintage object selected for the collection.'),
    condition: equivalent(lot.condition, 'Demo illustration depicts general wear; physical condition and operation are unknown.', 'Condition and operation have not been assessed.'),
    story: lot.story === null ? null : equivalent(lot.story, "Project-authored demo story: an imagined object from a collector's shelf. No provenance has been independently verified.", 'Provenance information is unavailable.'),
    images: lot.images.map(image => identity(image.path) === title ? imageDisplay(image, title) : image),
    specifications: lot.specifications.map(s => s.name === 'Representation' && s.value === 'Project-authored SVG illustration' ? { ...s, value: 'Object study' } : s),
    fulfillment: fulfillmentDisplay(lot.fulfillment) }
}
export function displayEvent<T extends PublicEvent>(event: T): T {
  if (!event.demo || !/^\/site-assets\/catalog\/event-[123]\.svg$/.test(event.cover_path)) return event
  const title = ['Analog Memories', 'Objects of Time', 'The Listening Room'][Number(event.cover_path.match(/event-(\d)/)![1]) - 1]!
  return { ...event,
    theme: equivalent(event.theme, 'Illustrative vintage collection', 'A collection of vintage objects'),
    description: equivalent(event.description, 'Demonstration content. These illustrations are fictional; no authenticity or sale is asserted.', 'Explore objects, materials and details from another time. Provenance information is unavailable.'),
    cover_alt: equivalent(event.cover_alt, title + ' demonstration illustration', title + ' collection'),
    lots: event.lots.map(displayLot) }
}
export function displayOrder<T extends Order>(order: T): T {
  const s = order.snapshot, title = s.lot_demo ? identity(s.image.path) : undefined
  if (!title) return order
  const f = fulfillmentDisplay(s.fulfillment)
  const instructions = (v: string) => equivalent(equivalent(v, s.fulfillment.shipping_instructions, f.shipping_instructions), s.fulfillment.pickup_instructions, f.pickup_instructions)
  return { ...order, snapshot: { ...s,
    condition: equivalent(s.condition, 'Demo illustration depicts general wear; physical condition and operation are unknown.', 'Condition and operation have not been assessed.'),
    image: imageDisplay(s.image, title), fulfillment: f },
    options: order.options.map(o => ({ ...o, instructions: instructions(o.instructions) })),
    total: order.total ? { ...order.total, instructions: instructions(order.total.instructions) } : null }
}

export function displaySummary<T extends Summary>(summary: T): T {
  const title = summary.lot_demo ? identity(summary.image.path) : undefined
  return title ? { ...summary, image: imageDisplay(summary.image, title) } : summary
}
