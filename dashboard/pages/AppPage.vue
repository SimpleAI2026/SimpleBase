<template>
  <div>
    <ol class="breadcrumb">
      <li><router-link to="/">Apps</router-link> </li>
      <li class="active">{{ app.name }}</li>
    </ol>
    <br />


    <h1 class="page-header">
      {{ app.name }}

      <div class="pull-right">
        <button id="openCreateFn" class="btn btn-default" @click="openAddFn()"><i class="fa fa-plus"></i> Add
          Function</button>
      </div>
    </h1>

    <br />

    <div class="row">
      <div class="col-md-12 col-lg-10">
        <table id="fnTable" class="table table-striped">
          <thead v-bind:class="{ transparent: fns.length == 0 }">
            <th>Name</th>
            <th>Image</th>
            <th>Memory</th>
            <th>Timeout</th>
            <th>Idle Timeout</th>
            <th width="140">Actions</th>
          </thead>
          <tbody>
            <tr :name="fn.name" v-for="fn in fns">
              <td name="name">{{ fn.name }}</td>
              <td name="image">{{ fn.image }}</td>
              <td name="memory">{{ fn.memory }} MB</td>
              <td name="timeout">{{ fn.timeout }}</td>
              <td name="idleTimeout">{{ fn.idle_timeout }}</td>
              <td>
                <div class="toolbar">

                  <div class="btn-group">
                    <button name="runFn" class="btn btn-default btn-sm" @click.prevent="openRunFn(fn)"
                      title="Run Function"><i class="fa fa-play"></i> Run Function</button>
                    <button name="openMoreOptions" type="button" class="btn btn-default dropdown-toggle btn-sm"
                      data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
                      <span class="caret"></span>
                      <span class="sr-only">Toggle Dropdown</span>
                    </button>
                    <ul class="dropdown-menu dropdown-menu-right">
                      <li>
                        <a name="openEditFn" href="#" @click.prevent="openEditFn(fn)" title="Edit Function">
                          <i class="fa fa-gear"></i> Edit Function
                        </a>
                        <a name="deleteFn" href="#" @click.prevent="deleteFn(fn)" class="text-danger"
                          title="Delete Function">
                          <i class="fa fa-times"></i> Delete Function
                        </a>
                      </li>
                    </ul>
                  </div>
                </div>
              </td>
            </tr>
            <tr v-if="fnsLoaded && fns.length == 0">
              <td colspan="99" class="no-matches">
                <div>
                  No Functions
                </div>
              </td>
            </tr>
            <tr v-if="!fnsLoaded">
              <td colspan="99" class="no-matches">
                <div>Loading...</div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <h3 class="page-header">
      Statistics
      <div class="pull-right">
        <label class="btn btn-default checkbox-inline" style="padding-left:30px"><!-- TODO style this properly -->
          <input type="checkbox" v-model="isChecked" @change="appPageAutoRefreshButtonClicked">Auto refresh
        </label>
      </div>
    </h3>
    <stats-charts :showAppCharts=true :stats="stats" :statshistory="statshistory" :appid="appid"
      :fns="fns"></stats-charts>

    <fn-function-form :app="app"></fn-function-form>
    <fn-run-function :app="app"></fn-run-function>
  </div>
</template>

<script>
import StatsCharts from '../components/StatsCharts.vue'
import FnFunctionForm from '../components/FnFunctionForm.vue';
import FnRunFunction from '../components/FnRunFunction.vue';
import { eventBus } from '../main';
import { defaultErrorHandler, getAuthToken } from '../libs/util';

export default {
  props: ['apps', 'stats', 'statshistory', 'autorefresh'],
  data: function () {
    return {
      app: {},
      fns: [],
      fnsLoaded: false,
      isChecked: true,
      appid: "",
    }
  },
  components: {
    StatsCharts,
    FnFunctionForm,
    FnRunFunction
  },
  methods: {
    appPageAutoRefreshButtonClicked: function (checkboxElem) {
      if (checkboxElem.currentTarget.checked) {
        eventBus.$emit('startAutoRefreshStats');
      } else {
        eventBus.$emit('stopAutoRefreshStats');
      }
    },
    openAddFn: function () {
      eventBus.$emit('openAddFn');
    },
    openEditFn: function (fn) {
      eventBus.$emit('openEditFn', fn);
    },
    openRunFn: function (fn) {
      eventBus.$emit('openRunFn', fn);
    },
    loadFns: function () {
      var t = this;
      $.ajax({
        headers: { 'Authorization': getAuthToken() },
        url: '/api/fns/?app_id=' + encodeURIComponent(t.app.id),
        dataType: 'json',
        success: function (fns) {
          t.fns = fns;
          t.fnsLoaded = true;
        },
        error: defaultErrorHandler
      })
    },
    loadApp: function (appID, cb) {
      var t = this;
      $.ajax({
        headers: { 'Authorization': getAuthToken() },
        url: '/api/apps/' + encodeURIComponent(appID),
        dataType: 'json',
        success: (app) => { t.app = app; if (cb) { cb() } },
        error: defaultErrorHandler
      })
    },
    deleteFn: function (fn) {
      if (confirm('Are you sure you want to delete function ' + fn.name + '?')) {
        var t = this;
        $.ajax({
          headers: { 'Authorization': getAuthToken() },
          url: '/api/fns/' + encodeURIComponent(fn.id),
          method: 'DELETE',
          dataType: 'json',
          success: (app) => { t.loadFns() },
          error: defaultErrorHandler
        })
      }
    },
    setTitle: function () {
      document.title = this.app.name + " | Functions UI";
    },
    appLoaded: function () {
      this.appid = this.app.id;
      this.fns = [];
      this.fnsLoaded = false;
      this.loadFns();
      this.setTitle();
      eventBus.$emit('AppOpened', this.app);
    },
    appSwitched: function () {
      this.loadApp(this.$route.params.appid, () => { this.appLoaded(); });
    }
  },
  watch: {
    '$route': 'appSwitched'
  },
  beforeRouteEnter(to, from, next) {
    // access to component instance via `vm`
    next(vm => {
      if (vm.apps) {
        vm.app = _.find(vm.apps, (app) => { return app.id == to.params.appid });
        vm.appLoaded();
      } else {
        vm.loadApp(to.params.appid, () => { vm.appLoaded(); });
      }
    })
  },
  destroyed: function () {
    eventBus.$emit('AppClosed');
  },
  created: function () {
    eventBus.$on('FnAdded', (fn) => {
      this.loadFns()
    });
    eventBus.$on('FnUpdated', (fn) => {
      this.loadFns()
    });
    this.isChecked = this.autorefresh;
    if (this.autorefresh) {
      eventBus.$emit('startAutoRefreshStats');
    }
  }
}
</script>

<style></style>
