import { api } from './api'

export interface Catalog {
  id: string
  meta_catalog_id: string
  whatsapp_account: string
  name: string
  is_active: boolean
  product_count: number
  created_at?: string
  updated_at?: string
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
  availability: string
  condition: string
  is_active: boolean
  created_at?: string
  updated_at?: string
}

export async function listCatalogs(account: string): Promise<Catalog[]> {
  const res = await api.get('/catalogs', { params: account ? { whatsapp_account: account } : {} })
  return res.data?.catalogs ?? []
}

export async function createCatalog(account: string, name: string): Promise<void> {
  await api.post('/catalogs', { whatsapp_account: account, name })
}

export interface CatalogSyncResult {
  synced: number
  total: number
  products_synced: number
}

export async function syncCatalogs(account: string): Promise<CatalogSyncResult> {
  const res = await api.post('/catalogs/sync', { whatsapp_account: account })
  return res.data
}

export async function deleteCatalog(id: string): Promise<void> {
  await api.delete(`/catalogs/${id}`)
}

export async function activateCatalog(id: string): Promise<void> {
  await api.post(`/catalogs/${id}/activate`)
}

export async function listProducts(catalogId: string): Promise<Product[]> {
  const res = await api.get(`/catalogs/${catalogId}/products`)
  return res.data?.products ?? []
}

export interface ProductInput {
  name: string
  description?: string
  price: number | null // major units; null/0 means price is not displayed
  currency: string
  url: string
  image_url: string
  retailer_id: string
  availability?: string
  condition?: string
}

export async function uploadCatalogImage(file: File): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const res = await api.post('/catalog-images', form, {
    headers: { 'Content-Type': undefined as unknown as string }
  })
  return res.data?.url ?? ''
}

export async function createProduct(catalogId: string, input: ProductInput): Promise<void> {
  await api.post(`/catalogs/${catalogId}/products`, productPayload(input))
}

export async function updateProduct(productId: string, input: ProductInput): Promise<void> {
  await api.put(`/products/${productId}`, productPayload(input))
}

export async function deleteProduct(productId: string): Promise<void> {
  await api.delete(`/products/${productId}`)
}

function productPayload(input: ProductInput) {
  return {
    name: input.name,
    description: input.description || '',
    price: Math.round((input.price || 0) * 100), // to cents; 0 omits price in Meta
    currency: input.currency || 'TRY',
    url: input.url || '',
    image_url: input.image_url || '',
    retailer_id: input.retailer_id || '',
    availability: input.availability || 'in stock',
    condition: input.condition || 'new'
  }
}
