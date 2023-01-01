# Schema

## Driver

Driver {
    Name = mysql
    Databases = XXX
    Uri = XXX
    User = root
    Password = root
}

## Init

Init {
    Create table xxx1(

    );

    Create table xxx2(

    );

    Alter table XXXX1;

    Alter table XXXX2;
}

## Select

SelectName1 {
    SQL {

    }
} 

SelectName1 { 
    Select {
        id
        name
    } From XXX
}

SelectName2 {
    Select {
        id
        name
    } 
    From XXX 
    JOIN XXX1
    Using id
}

SelectName3 { 
    Select {
        id
        name
    } 
    From XXX 
    FullJoin XXX1
    Using id
}

## Insert

InsertName1 {
    SQL {

    }
}

InsertName1 {
    InsertInto XXX {
        people = ?
        name = ?
        John = ?
    }
} 

## Update

UpdateName1 {
    SQL {

    }   
}

UpdateName1 {
    Update XXX Set {
        people = ?
        name = ?
        John = ?
    } Where {
        people = ?
    }
} 

UpdateName2 {
    Update XXX Set {
        people = ?
        name = ?
        John = ?
    } Where {
        people = ?
        OR
        people = ?
    }
}

## Delete

DeleteName1 {
    SQL {

    }
} 

DeleteName1 {
    DeleteFrom XXXX Where {
        people = ?
    }
}

## 注释

使用 // /*** ***/