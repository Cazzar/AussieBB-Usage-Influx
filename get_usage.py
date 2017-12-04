"""fuckoff"""
from datetime import datetime
import os
import time
import xml.etree.ElementTree
import requests
from influxdb import InfluxDBClient

db = InfluxDBClient(
    os.environ['INFLUX_HOST'],
    os.environ['INFLUX_PORT'],
    os.environ['INFLUX_USER'],
    os.environ['INFLUX_PASS'],
    os.environ['INFLUX_DB'])

def http_get(user, passw):
    """fuckoff"""
    URL = "https://my.aussiebroadband.com.au/usage.php?xml=yes"
    data = {
        'login_username': user,
        'login_password': passw
    }
    resp = requests.post(URL, data)
    tree = xml.etree.ElementTree.fromstring(resp.content)
    data = dict()

    for child in tree:
        data[child.tag] = child.text

    return {
        'download':    int(data['down1']),
        'upload':      int(data['up1']),
        'allowance':   int(data['allowance1_mb']) * 1000 * 1000,
        'left':        int(data['left1']),
        'lastupdated': datetime.strptime(data['lastupdated'], '%Y-%m-%d %H:%M:%S'),
        'rollover':    int(data['rollover'])
    }

    """Please, no"""
if not os.environ['INFLUX_DB'] in db.get_list_database():
        db.create_database(os.environ['INFLUX_DB'])

while True:
    users = os.environ['MYAUSSIE_USER'].split(',')
    passes = os.environ['MYAUSSIE_PASS'].split(',')
    json_body = [
        
    ]
    now = datetime.now()
    for i in range(len(users)):
        data = http_get(users[i], passes[i])
        json_body.append({
            'measurement': 'usage',
            'tags': {
                'user': users[i]
            },
            'time': now,
            'fields': {
                'download': data['download'],
                'upload': data['upload'],
                'allowance': data['allowance'],
                'left': data['left'],
                'rollover': data['rollover']
            }
        })

    print(json_body)
    db.write_points(json_body)
    time.sleep(15 * 60)

