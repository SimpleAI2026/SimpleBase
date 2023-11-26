<template>
  <div>

    <router-link class="navbar-brand" to="/">
      <!-- See also banner for small devices in index.html -->
      <img class="navbar-brand" src="/images/icons/fn_transparent.png">
    </router-link>
    <div>
      <div v-if="!loggedIn">
        <button type="button" class="btn btn-default" @click='login' :disabled="loggingin">
          Login
        </button>
        <input style="color:grey;" type="text" placeholder="FN_TOKEN" v-model="fn_token"></input>
      </div>
      <button type="button" class="btn btn-default" v-if="loggedIn" @click='logout'>
        Logout
      </button>
    </div>

    <ul class="nav nav-sidebar" v-if="app">
      <li :class="{ active: app && app.id == a.id }" v-for="a in apps" :key="a.id">
        <router-link :to="'/app/' + encodeURIComponent(a.id)">
          {{ a.name }}
          <span class="sr-only" v-if="app && app.id == a.id">(current)</span>
        </router-link>
      </li>
    </ul>


    <fn-welcome-section v-if="!app"></fn-welcome-section>
  </div>
</template>

<script>
import { eventBus } from '../main';
import FnWelcomeSection from '../components/FnWelcomeSection.vue';
import { defaultErrorHandler } from '../libs/util';

export default {
  props: ['apps'],
  data: function () {
    return {
      app: null,
      loggedIn: (window.localStorage.FN_TOKEN !== "" && window.localStorage.FN_TOKEN !== undefined),
      loggingin: false,
      fn_token: ""
    }
  },
  methods: {
    login: function () {
      var t = this
      t.loggingin = true
      $.ajax({
        headers: { 'Authorization': 'Bearer ' + t.fn_token },
        url: '/api/apps/',
        method: 'GET',
        contentType: "application/json",
        dataType: 'json',
        success: (res) => {
          t.loggingin = false;
          window.localStorage.setItem('FN_TOKEN', 'Bearer ' + t.fn_token);
          t.fn_token = "";
          eventBus.$emit('LoggedIn');
          eventBus.$emit('NotificationClear');
        },
        error: function (jqXHR, textStatus, errorThrown) {
          t.loggingin = false;
          t.fn_token = "";
          defaultErrorHandler(jqXHR);
        }
      })
    },
    logout: function () {
      window.localStorage.removeItem('FN_TOKEN')
      eventBus.$emit('LoggedOut');
    }
  },
  components: {
    FnWelcomeSection
  },
  created: function () {
    eventBus.$on('AppOpened', (app) => {
      this.app = app;
    });
    eventBus.$on('AppClosed', () => {
      this.app = null;
    });
    eventBus.$on('LoggedOut', () => {
      this.loggedIn = false;
    });
    eventBus.$on('LoggedIn', () => {
      this.loggedIn = true;
    });
  }
}
</script>

<style></style>
