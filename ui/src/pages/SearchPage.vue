<template>
  <q-page class="row items-center justify-evenly">
    <SongsList :filter="'sc_' + search" />
  </q-page>
</template>

<script lang="ts">
import { defineComponent } from 'vue';
import SongsList from '../components/SongsList.vue';

export default defineComponent({
  name: 'SearchPage',
	components: {SongsList},
	props: {
		search: {
			type: String,
			required: true,
		},
	},
});
</script>
