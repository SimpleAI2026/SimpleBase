<template>
    <div>
        <fn-notification></fn-notification>

        <nav class="navbar navbar-inverse navbar-fixed-top">
            <div class="container-fluid">
                <div class="navbar-header">
                    <button type="button" class="navbar-toggle collapsed" data-toggle="collapse" data-target="#navbar"
                        aria-expanded="false" aria-controls="navbar">
                        <span class="sr-only">Toggle navigation</span>
                        <span class="icon-bar"></span>
                        <span class="icon-bar"></span>
                        <span class="icon-bar"></span>
                    </button>
                    <a href="#">
                        <!-- See also sidebar banner for large devices in FNSidebar.vue -->
                        <img class="navbar-brand" src="./images/icons/fn_transparent.png">
                    </a>
                </div>
                <div id="navbar" class="navbar-collapse collapse" aria-expanded="false" style="height: 1px;">
                    <ul class="nav navbar-nav navbar-right">
                        <li>
                            <a href="https://github.com/fnproject/fn#quickstart" target="_blank">
                                <i class="fa fa-bolt"></i> Quick Start
                            </a>
                        </li>
                        <li><a href="http://petstore.swagger.io/?url=https://raw.githubusercontent.com/fnproject/fn/master/docs/swagger.yml"
                                target="_blank">
                                <i class="fa fa-file-text-o"></i> Fn API</a>
                        </li>
                    </ul>
                </div>
            </div>
        </nav>

        <div class="container-fluid">
            <div class="row">
                <div class="col-sm-4 col-md-4 col-lg-2 sidebar">
                    <fn-sidebar :apps="apps"></fn-sidebar>
                </div>
                <div class="col-sm-8 col-sm-offset-4 col-md-9 col-md-offset-3 col-lg-10 col-lg-offset-2 main">
                    <div>
                        <router-view :apps="apps" :stats="stats" :statshistory="statshistory"
                            :autorefresh="autorefresh"></router-view>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>
<script>
import FnSidebar from './components/FnSidebar.vue';
import FnNotification from './components/FnNotification.vue';

export default {
    props: ['apps', 'stats', 'statshistory', 'autorefresh'],
    components: {
        FnSidebar,
        FnNotification,
    },
    methods: {
    }
}
</script>