import { api } from './api'

export interface Catalog {
  id: string
  meta_catalog_id: string
  whatsapp_account: string
  name: string
  is_active: boolean
  product_count: number
}

export interface Product {
  id: string
  meta_product_id: string
  name: string
  description: string
  price: number // cents
  currency: string
  url: string
  image_url: string
  retailer_id: string
  is_active: boolean
}

export async function listCatalogs(account: string): Promise<Catalog[]> {
  const res = await api.get('/catalogs', { params: account ? { whatsapp_account: account } : {} })
  return res.data?.catalogs ?? []
}

export async function createCatalog(account: string, name: string): Promise<void> {
  await api.post('/catalogs', { whatsapp_account: account, name })
}

export async function syncCatalogs(account: string): Promise<void> {
  await api.post('/catalogs/sync', { whatsapp_account: account })
}

export async function deleteCatalog(id: string): Promise<void> {
  await api.delete(`/catalogs/${id}`)
}

export async function listProducts(catalogId: string): Promise<Product[]> {
  const res = await api.get(`/catalogs/${catalogId}/products`)
  return res.data?.products ?? []
}

export interface ProductInput {
  name: string
  description?: string
  price: number // major units (e.g. 100.50) — converted to cents here
  currency: string
  url?: string
  image_url?: string
  retailer_id?: string
}

export async function createProduct(catalogId: string, input: ProductInput): Promise<void> {
  await api.post(`/catalogs/${catalogId}/products`, {
    name: input.name,
    description: input.description || '',
    price: Math.round(input.price * 100), // to cents
    currency: input.currency || 'TRY',
    url: input.url || '',
    image_url: input.image_url || '',
    retailer_id: input.retailer_id || ''
  })
}
