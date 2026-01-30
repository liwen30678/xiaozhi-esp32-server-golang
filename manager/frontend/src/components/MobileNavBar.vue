<template>
  <van-nav-bar
    :title="title"
    :left-arrow="showBack"
    :left-text="leftText"
    :right-text="rightText"
    @click-left="handleLeftClick"
    @click-right="handleRightClick"
    fixed
    placeholder
    safe-area-inset-top
    class="mobile-nav-bar"
  >
    <template #right v-if="$slots.right">
      <slot name="right"></slot>
    </template>
  </van-nav-bar>
</template>

<script setup>
import { useRouter } from 'vue-router'

const props = defineProps({
  title: {
    type: String,
    default: '小智管理系统'
  },
  showBack: {
    type: Boolean,
    default: true
  },
  leftText: {
    type: String,
    default: ''
  },
  rightText: {
    type: String,
    default: ''
  }
})

const emit = defineEmits(['click-left', 'click-right'])

const router = useRouter()

const handleLeftClick = () => {
  if (props.showBack) {
    router.back()
  }
  emit('click-left')
}

const handleRightClick = () => {
  emit('click-right')
}
</script>

<style scoped>
.mobile-nav-bar {
  background-color: #409EFF;
  color: white;
}

:deep(.van-nav-bar__title) {
  color: white;
  font-weight: 500;
}

:deep(.van-nav-bar__arrow) {
  color: white;
}

:deep(.van-nav-bar__text) {
  color: white;
}
</style>
