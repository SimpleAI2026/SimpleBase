<template>
  <div>

    <ol class="breadcrumb">
      <li class="active">Apps</li>
    </ol>
    <br>
    <h1 class="page-header">
      Dashboard

      <div class="pull-right">
        <button id="openCreateApp" class="btn btn-default" @click="openAddApp">
          <i class="fa fa-plus"></i>&nbsp;
          Create App
        </button>
      </div>
    </h1>

    <h3 class="page-header">Applications</h3>

    <div class="row">
      <div class="col-md-12 col-lg-10">
        <!-- <div class="table-responsive"> -->
        <div>
          <table id="appsTable" class="table table-striped">
            <thead v-bind:class="{ transparent: !apps || apps.length == 0 }">
              <th>Name</th>
              <th width="120">Actions</th>
            </thead>
            <tbody>
              <tr :name="app.name" v-for="app in apps">
                <td>
                  <router-link :to="'/app/' + encodeURIComponent(app.id)" name="appLink">{{ app.name }}</router-link>
                </td>
                <td>
                  <div class="toolbar">

                    <div class="btn-group">
                      <button name="openEditApp" class="btn btn-default btn-sm" @click.prevent="openEditApp(app)"
                        title="Edit App"><i class="fa fa-gear"></i> Edit</button>
                      <button name="openMoreOptions" type="button" class="btn btn-default dropdown-toggle btn-sm"
                        data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
                        <span class="caret"></span>
                        <span class="sr-only">Toggle Dropdown</span>
                      </button>
                      <ul class="dropdown-menu dropdown-menu-right">
                        <li>
                          <a name="deleteApp" href="#" @click.prevent="deleteApp(app)" class="text-danger"
                            title="Delete App">
                            <i class="fa fa-times"></i> Delete App
                          </a>
                        </li>
                      </ul>
                    </div>

                  </div>
                </td>
              </tr>
              <tr v-if="apps && apps.length == 0">
                <td colspan="99" class="no-matches">
                  <div>No Apps</div>
                </td>
              </tr>
              <tr v-if="!apps">
                <td colspan="99" class="no-matches">
                  <div>Loading...</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>


    <h3 class="page-header">
      Statistics
      <div class="pull-right">
        <label class="btn btn-default checkbox-inline" style="padding-left:30px"><!-- TODO style this properly -->
          <input type="checkbox" v-model="isChecked" @change="indexPageAutoRefreshButtonClicked">Auto refresh
        </label>
      </div>
    </h3>

    <stats-charts :showGeneralCharts=true :stats="stats" :statshistory="statshistory"></stats-charts>

    <!-- Define the "Create New App" modal -->
    <fn-app-form></fn-app-form>
  </div>
</template>

<script>
import FnAppForm from '../components/FnAppForm.vue';
import StatsCharts from '../components/StatsCharts.vue'
import { eventBus } from '../main';
import { defaultErrorHandler, getAuthToken } from '../libs/util';

export default {
  props: ['apps', 'stats', 'statshistory', 'autorefresh'],
  data() {
    return {
      isChecked: true,
    }
  },
  components: {
    FnAppForm,
    StatsCharts
  },
  methods: {
    indexPageAutoRefreshButtonClicked: function (checkboxElem) {
      if (checkboxElem.currentTarget.checked) {
        eventBus.$emit('startAutoRefreshStats');
      } else {
        eventBus.$emit('stopAutoRefreshStats');
      }
    },
    openAddApp: function () {
      eventBus.$emit('openAddApp');
    },
    openEditApp: function (app) {
      eventBus.$emit('openEditApp', app);
    },
    deleteApp: function (app) {
      if (confirm('Are you sure you want to delete app ' + app.name + '?')) {
        var t = this;
        $.ajax({
          headers: { 'Authorization': getAuthToken() },
          url: '/api/apps/' + encodeURIComponent(app.id),
          method: 'DELETE',
          dataType: 'json',
          success: (app) => { eventBus.$emit('AppDeleted', app) },
          error: defaultErrorHandler
        })
      }
    }
  },
  created: function () {
    document.title = "Functions UI";
    this.isChecked = this.autorefresh;
    if (this.autorefresh) {
      eventBus.$emit('startAutoRefreshStats');
    }
  }
}
</script>

<style></style>
