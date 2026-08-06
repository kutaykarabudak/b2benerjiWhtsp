<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { TagBadge } from '@/components/ui/tag-badge'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { contactsService, accountsService, type Tag } from '@/services/api'
import { useTagsStore } from '@/stores/tags'
import { toast } from 'vue-sonner'
import { Loader2, Check, ChevronsUpDown, X } from 'lucide-vue-next'
import { getErrorMessage } from '@/lib/api-utils'
import { getTagColorClass } from '@/lib/constants'

const { t } = useI18n()
const tagsStore = useTagsStore()

interface Props {
  open: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:open': [value: boolean]
  'created': [contact: any]
}>()

interface ContactFormData {
  phone_number: string
  profile_name: string
  whatsapp_account: string
  tags: string[]
  email: string; company_name: string; tax_office: string; tax_number: string
  address: string; city: string; district: string; postal_code: string
  purchase_score: number; has_purchased: boolean
}

const defaultFormData: ContactFormData = { phone_number: '', profile_name: '', whatsapp_account: '', tags: [], email: '', company_name: '', tax_office: '', tax_number: '', address: '', city: '', district: '', postal_code: '', purchase_score: 0, has_purchased: false }

const formData = ref<ContactFormData>({ ...defaultFormData })
const isSubmitting = ref(false)
const tagSelectorOpen = ref(false)
const availableTags = ref<Tag[]>([])
const availableAccounts = ref<{ id: string; name: string; phone_number: string }[]>([])

watch(() => props.open, (isOpen) => {
  if (isOpen) {
    formData.value = { ...defaultFormData }
    fetchTags()
    fetchAccounts()
  }
})

async function fetchTags() {
  try {
    const response = await tagsStore.fetchTags({ limit: 100 })
    availableTags.value = response.tags
  } catch {
    // Silently fail - tags are optional
  }
}

async function fetchAccounts() {
  try {
    const response = await accountsService.list()
    const data = response.data as any
    const responseData = data.data || data
    availableAccounts.value = responseData.accounts || []
  } catch (error) {
    // Silently fail - accounts are optional
  }
}

async function saveContact() {
  if (!formData.value.phone_number.trim()) {
    toast.error(t('contacts.phoneRequired'))
    return
  }
  isSubmitting.value = true
  try {
    const response = await contactsService.create({
      phone_number: formData.value.phone_number.trim(),
      profile_name: formData.value.profile_name.trim() || undefined,
      whatsapp_account: formData.value.whatsapp_account || undefined,
      tags: formData.value.tags.length > 0 ? formData.value.tags : undefined
      , email: formData.value.email, company_name: formData.value.company_name,
      tax_office: formData.value.tax_office, tax_number: formData.value.tax_number,
      address: formData.value.address, city: formData.value.city, district: formData.value.district,
      postal_code: formData.value.postal_code, purchase_score: Number(formData.value.purchase_score),
      has_purchased: formData.value.has_purchased
    })
    const contact = response.data?.data || response.data
    toast.success(t('common.createdSuccess', { resource: t('resources.Contact') }))
    emit('update:open', false)
    emit('created', contact)
  } catch (error) {
    toast.error(getErrorMessage(error, t('common.failedCreate', { resource: t('resources.contact') })))
  } finally {
    isSubmitting.value = false
  }
}

function toggleTag(tagName: string) {
  const index = formData.value.tags.indexOf(tagName)
  if (index === -1) {
    formData.value.tags.push(tagName)
  } else {
    formData.value.tags.splice(index, 1)
  }
}

function removeTag(tagName: string) {
  formData.value.tags = formData.value.tags.filter(t => t !== tagName)
}

function isTagSelected(tagName: string): boolean {
  return formData.value.tags.includes(tagName)
}

function getTagDetails(tagName: string): Tag | undefined {
  return availableTags.value.find(t => t.name === tagName)
}

function closeDialog() {
  emit('update:open', false)
}
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="sm:max-w-2xl max-h-[90vh] overflow-y-auto">
      <DialogHeader>
        <DialogTitle>{{ $t('contacts.createTitle') }}</DialogTitle>
        <DialogDescription>{{ $t('contacts.createDesc') }}</DialogDescription>
      </DialogHeader>
      <div class="space-y-4 py-4">
        <div class="space-y-2">
          <Label>{{ $t('contacts.phoneNumber') }} <span class="text-destructive">*</span></Label>
          <Input v-model="formData.phone_number" :placeholder="$t('contacts.phonePlaceholder')" />
          <p class="text-xs text-muted-foreground">{{ $t('contacts.phoneHint') }}</p>
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div class="space-y-2"><Label>E-posta</Label><Input v-model="formData.email" type="email" /></div>
          <div class="space-y-2"><Label>Firma / Ünvan</Label><Input v-model="formData.company_name" /></div>
          <div class="space-y-2"><Label>Vergi dairesi</Label><Input v-model="formData.tax_office" /></div>
          <div class="space-y-2"><Label>Vergi / T.C. kimlik no</Label><Input v-model="formData.tax_number" /></div>
          <div class="space-y-2"><Label>Şehir</Label><Input v-model="formData.city" /></div>
          <div class="space-y-2"><Label>İlçe</Label><Input v-model="formData.district" /></div>
          <div class="space-y-2"><Label>Posta kodu</Label><Input v-model="formData.postal_code" /></div>
          <div class="space-y-2"><Label>Satın alma puanı (0-100)</Label><Input v-model.number="formData.purchase_score" type="number" min="0" max="100" /></div>
        </div>
        <div class="space-y-2"><Label>Açık adres</Label><Textarea v-model="formData.address" rows="3" /></div>
        <div class="flex items-center justify-between rounded-lg border p-3">
          <div><Label>Daha önce satın alım yaptı</Label><p class="text-xs text-muted-foreground">CRM hedef kitle filtrelerinde kullanılır.</p></div>
          <Switch v-model:checked="formData.has_purchased" />
        </div>
        <div class="space-y-2">
          <Label>{{ $t('contacts.profileName') }}</Label>
          <Input v-model="formData.profile_name" :placeholder="$t('contacts.namePlaceholder')" />
        </div>
        <div v-if="availableAccounts.length > 0" class="space-y-2">
          <Label>{{ $t('contacts.whatsappAccount') }}</Label>
          <Select v-model="formData.whatsapp_account">
            <SelectTrigger>
              <SelectValue :placeholder="$t('contacts.selectAccount')" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="account in availableAccounts" :key="account.id" :value="account.name">
                {{ account.name }} ({{ account.phone_number }})
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div v-if="availableTags.length > 0" class="space-y-2">
          <Label>Kategoriler</Label>
          <Popover v-model:open="tagSelectorOpen">
            <PopoverTrigger as-child>
              <Button variant="outline" role="combobox" class="w-full justify-between">
                <span v-if="formData.tags.length === 0" class="text-muted-foreground">{{ $t('contacts.selectTags') }}</span>
                <span v-else>{{ formData.tags.length }} {{ $t('contacts.tagsSelected') }}</span>
                <ChevronsUpDown class="ml-2 h-4 w-4 shrink-0 opacity-50" />
              </Button>
            </PopoverTrigger>
            <PopoverContent class="w-[300px] p-0" @interact-outside="(e) => e.preventDefault()">
              <Command>
                <CommandInput :placeholder="$t('contacts.searchTags')" />
                <CommandList>
                  <CommandEmpty>{{ $t('contacts.noTagsFound') }}</CommandEmpty>
                  <CommandGroup>
                    <CommandItem
                      v-for="tag in availableTags"
                      :key="tag.name"
                      :value="tag.name"
                      class="flex items-center gap-2 cursor-pointer"
                      @select.prevent="toggleTag(tag.name)"
                    >
                      <div class="flex items-center gap-2 flex-1">
                        <span :class="['w-2 h-2 rounded-full', getTagColorClass(tag.color).split(' ')[0]]"></span>
                        <span>{{ tag.name }}</span>
                      </div>
                      <Check v-if="isTagSelected(tag.name)" class="h-4 w-4 text-primary" />
                    </CommandItem>
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
          <div v-if="formData.tags.length > 0" class="flex flex-wrap gap-1 mt-2">
            <TagBadge
              v-for="tagName in formData.tags"
              :key="tagName"
              :color="getTagDetails(tagName)?.color"
            >
              {{ tagName }}
              <button
                type="button"
                class="ml-1 rounded-full hover:bg-black/10 dark:hover:bg-white/10 p-0.5 transition-colors"
                @click.stop="removeTag(tagName)"
              >
                <X class="h-3 w-3" />
              </button>
            </TagBadge>
          </div>
        </div>
      </div>
      <div class="flex justify-end gap-2">
        <Button variant="outline" @click="closeDialog">{{ $t('common.cancel') }}</Button>
        <Button @click="saveContact" :disabled="isSubmitting">
          <Loader2 v-if="isSubmitting" class="h-4 w-4 mr-2 animate-spin" />
          {{ $t('common.create') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
