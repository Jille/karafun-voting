<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated>
      <q-toolbar>
        <q-toolbar-title class="row justify-between">
					<template v-if="!searching">
						<router-link class="text-white" to="/">Karafun</router-link>
						<q-btn icon="search" @click="searching = true" round flat />
					</template>
					<template v-else>
						<q-btn icon="chevron_left" @click="searching = false" round flat />
						<q-input v-model="search" debounce="300" clearable color="white" dense class="col-grow" />
					</template>
        </q-toolbar-title>
      </q-toolbar>
    </q-header>

    <q-page-container>
      <router-view />
    </q-page-container>
  </q-layout>
</template>

<script lang="ts">
import { defineComponent } from 'vue';

export default defineComponent({
  name: 'MainLayout',

  data() {
    return {
			searching: false,
			search: '',
    }
  },

  watch: {
		search(str: string) {
			if(!str) {
				return;
			}
			this.$router.replace({name: 'search', params: { search: str }});
		},
  }
});
</script>
